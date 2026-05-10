package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/giantswarm/mcp-kubernetes/internal/instrumentation"
	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/logging"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/giantswarm/mcp-kubernetes/internal/tools"
	"github.com/giantswarm/mcp-kubernetes/internal/tools/output"
)

// recordK8sOperation records metrics for a Kubernetes operation.
// Delegates to ServerContext which handles nil checks internally.
func recordK8sOperation(ctx context.Context, sc *server.ServerContext, clusterName, operation, resourceType, namespace, status string, duration time.Duration) {
	sc.RecordK8sOperation(ctx, clusterName, operation, resourceType, namespace, status, duration)
}

// checkMutatingOperation is a convenience wrapper around tools.CheckMutatingOperation.
// It verifies if a mutating operation is allowed given the current server configuration.
func checkMutatingOperation(sc *server.ServerContext, operation string) *mcp.CallToolResult {
	return tools.CheckMutatingOperation(sc, operation)
}

// handleGetResource handles kubectl get operations
func handleGetResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(args)

	kubeContext, _ := args["kubeContext"].(string)
	apiGroup, _ := args["apiGroup"].(string)

	namespace, _ := args["namespace"].(string)
	// Follow kubectl behavior: if no namespace specified, use "default".
	// For cluster-scoped resources, the Kubernetes API ignores the namespace.
	if namespace == "" {
		namespace = k8s.DefaultNamespace
	}

	resourceType, ok := args["resourceType"].(string)
	if !ok || resourceType == "" {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	name, ok := args["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	// Output format mirrors kubernetes_list: slim (default) and normal go through
	// the server-configured slim processor; wide returns the full manifest.
	outputFormat, _ := args["output"].(string)

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	getResponse, err := k8sClient.Get(ctx, kubeContext, namespace, resourceType, apiGroup, name)
	duration := time.Since(start)

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationGet, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(tools.FormatK8sError("Failed to get resource", err, client.User())), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationGet, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Apply output processing (slim output, secret masking)
	processor := getOutputProcessorForFormat(sc, outputFormat)
	processedObj, err := output.ProcessSingleRuntimeObject(processor, getResponse.Resource)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to process resource: %v", err)), nil
	}

	// Build response with metadata
	response := map[string]interface{}{
		"resource": processedObj,
		"_meta":    getResponse.Meta,
	}

	// Convert the response to JSON for output
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal resource: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// getOutputProcessor creates an output processor from server context
// configuration, using the server-configured slim setting. Equivalent to
// getOutputProcessorForFormat(sc, "").
func getOutputProcessor(sc *server.ServerContext) *output.Processor {
	return getOutputProcessorForFormat(sc, "")
}

// getOutputProcessorForFormat builds an output processor that honours the
// per-call output format, while preserving server-level secret masking.
//
// Format semantics (matches docs/read-tools-arguments.md):
//
//   - "wide" / "full": SlimOutput=false, KindShaping=false.
//     Returns the full manifest, with only secret masking applied.
//   - "normal": SlimOutput=true, KindShaping=false.
//     Generic blacklist exclusion (managedFields, last-applied-configuration,
//     transition timestamps, ...) but no Kind-aware shaping.
//   - "slim" (and empty/default): SlimOutput=true, KindShaping=true.
//     Blacklist exclusion + per-Kind shaping (HelmRelease drops spec.values
//     and status.history; Deployment / StatefulSet / DaemonSet collapse long
//     container env lists). This is the LLM-friendly default per #410.
//
// Secret masking is always driven by the server-level MaskSecrets setting
// and is never disabled by output format — every read tool honours that
// contract uniformly.
func getOutputProcessorForFormat(sc *server.ServerContext, outputFormat string) *output.Processor {
	outputCfg := sc.OutputConfig()
	slim := outputCfg.SlimOutput
	kindShaping := slim
	switch outputFormat {
	case "wide", "full":
		slim = false
		kindShaping = false
	case "normal":
		kindShaping = false
	case "slim", "":
		// keep slim from server config; kindShaping mirrors slim so a
		// server with SlimOutput=false still returns wide for slim.
	}
	cfg := &output.Config{
		MaxItems:         outputCfg.MaxItems,
		MaxClusters:      outputCfg.MaxClusters,
		MaxResponseBytes: outputCfg.MaxResponseBytes,
		SlimOutput:       slim,
		KindShaping:      kindShaping,
		MaskSecrets:      outputCfg.MaskSecrets,
		SummaryThreshold: outputCfg.SummaryThreshold,
	}
	return output.NewProcessor(cfg)
}

// processorWithExtraExcluded returns a new Processor whose ExcludedFields
// list is the original cloned-and-extended with extra. Secret masking and
// every other config knob is preserved. Used by handleListResources to
// layer per-resourceType slim rules (e.g. Event-specific bookkeeping
// strips) on top of DefaultExcludedFields, without mutating the
// server-level config.
func processorWithExtraExcluded(p *output.Processor, extra []string) *output.Processor {
	cfg := p.Config().Clone()
	cfg.ExcludedFields = append(cfg.ExcludedFields, extra...)
	return output.NewProcessor(cfg)
}

// extraExcludedForResourceType returns slim-mode exclusion paths that only
// make sense for a specific resourceType. Returns nil when no extras
// apply (the default for non-events resources).
//
// resourceType matching mirrors how the kubectl API server's REST mapper
// resolves shortnames: we accept "events", "event", and "ev". Case is
// folded.
func extraExcludedForResourceType(resourceType string) []string {
	switch normalizeResourceType(resourceType) {
	case "event", "events", "ev":
		return eventListSlimFields
	}
	return nil
}

// compactItemsForResourceType applies value-conditional cleanup to slim-mode
// list items. Today only Events have a per-kind compactor: kubernetes
// always reports exactly one of (eventTime, firstTimestamp, lastTimestamp)
// as populated and the other two as null/empty, and `source: {}` is the
// permanent shape on events.k8s.io/v1 events. Stripping these only when
// they're null avoids fighting the (legitimate) signal-bearing copies.
//
// Items must be *unstructured.Unstructured (which is what
// ProcessRuntimeObjects returns). Non-unstructured items are skipped.
func compactItemsForResourceType(items []runtime.Object, resourceType string) {
	switch normalizeResourceType(resourceType) {
	case "event", "events", "ev":
		for _, it := range items {
			u, ok := it.(*unstructured.Unstructured)
			if !ok {
				continue
			}
			compactEventItem(u.Object)
		}
	}
}

// compactEventItem trims null timestamp siblings and empty source from a
// single event item map. See compactItemsForResourceType for why this lives
// outside the path-based exclusion list.
func compactEventItem(item map[string]any) {
	for _, k := range []string{"eventTime", "firstTimestamp", "lastTimestamp"} {
		if v, ok := item[k]; ok && v == nil {
			delete(item, k)
		}
	}
	if src, ok := item["source"].(map[string]any); ok && len(src) == 0 {
		delete(item, "source")
	}
}

// eventListSlimFields is the list-handler analogue of eventSlimFields. It
// strips the same per-event bookkeeping the describe handler strips
// (managedFields, uid, resourceVersion, selfLink, the involvedObject
// uid/resourceVersion/apiVersion duplication, and the empty
// reportingInstance), but deliberately preserves metadata.creationTimestamp
// and metadata.namespace because the list-summary code path
// (summarizeResource → obj.GetCreationTimestamp / GetNamespace) reads
// them to populate `age` and `namespace`. Stripping those breaks summary
// output for cross-namespace event listings without any byte saving on
// the much larger fullOutput path.
var eventListSlimFields = []string{
	"metadata.managedFields",
	"metadata.uid",
	"metadata.resourceVersion",
	"metadata.selfLink",
	"involvedObject.uid",
	"involvedObject.resourceVersion",
	"involvedObject.apiVersion",
	"reportingInstance",
}

// normalizeResourceType lower-cases a resourceType for switch comparison.
// Stays a free function (rather than inline strings.ToLower) so future
// shortname/aliasing rules have one place to land.
func normalizeResourceType(resourceType string) string {
	out := make([]byte, len(resourceType))
	for i := 0; i < len(resourceType); i++ {
		c := resourceType[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// slimMetadataMap applies "metadata.X"-anchored exclusion paths to a flat,
// metadata-shaped map (the convenience top-level metadata returned by
// kubernetes_describe, which duplicates labels / annotations / uid /
// resourceVersion / creationTimestamp / kind / apiVersion).
//
// The DefaultExcludedFields list is anchored at "metadata.…", so we wrap the
// map in a synthetic {"metadata": ...} envelope before handing it to
// SlimResource and unwrap after. Excluded paths that don't start with
// "metadata." are silently no-ops on this map, which is the correct
// behaviour: the convenience map only carries metadata-shaped fields.
//
// excludedFields comes from processor.Config().ExcludedFields, which is
// populated by output.NewProcessor → Config.Validate() when SlimOutput is
// enabled — callers therefore don't need to pass DefaultExcludedFields()
// explicitly.
func slimMetadataMap(m map[string]any, excludedFields []string) map[string]any {
	if len(m) == 0 {
		return m
	}
	envelope := map[string]any{"metadata": m}
	envelope = output.SlimResource(envelope, excludedFields)
	inner, ok := envelope["metadata"].(map[string]any)
	if !ok {
		return m
	}
	return inner
}

// handleListResources handles kubectl list operations
func handleListResources(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	handlerStart := time.Now()
	slog.Debug("list resources handler started", slog.String("method", request.Method))
	args := request.GetArguments()

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(args)

	kubeContext, _ := args["kubeContext"].(string)
	apiGroup, _ := args["apiGroup"].(string)

	resourceType, ok := args["resourceType"].(string)
	if !ok || resourceType == "" {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	namespace, _ := args["namespace"].(string)
	allNamespaces, _ := args["allNamespaces"].(bool)

	// Follow kubectl behavior: if no namespace specified, use "default".
	// For cluster-scoped resources, the Kubernetes API simply ignores the namespace.
	// This approach requires no static resource lists and works with any CRD.
	if !allNamespaces && namespace == "" {
		namespace = k8s.DefaultNamespace
	}

	labelSelector, _ := args["labelSelector"].(string)
	fieldSelector, _ := args["fieldSelector"].(string)

	// Client-side filtering parameter
	var filterCriteria FilterCriteria
	if filterArg, ok := args["filter"]; ok {
		filterMap, ok := filterArg.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("filter parameter must be an object/map"), nil
		}
		filterCriteria = filterMap
	}

	// New parameters for controlling output format
	fullOutput, _ := args["fullOutput"].(bool)
	includeLabels, _ := args["includeLabels"].(bool)
	includeAnnotations, _ := args["includeAnnotations"].(bool)

	// Summary mode parameter for fleet-scale operations
	summaryMode, _ := args["summary"].(bool)

	// Output format parameter (slim/normal/wide). Empty falls through to the
	// server-configured slim setting via getOutputProcessorForFormat.
	outputFormat, _ := args["output"].(string)

	// Per-resourceType slim extensions. The list path historically only
	// applies DefaultExcludedFields to items, but Events benefit from the
	// same per-event strip already used by handleDescribeResource — see
	// eventSlimFields in this file. Without this the LLM-default slim/list
	// of events still ships involvedObject.uid/resourceVersion/apiVersion,
	// reportingInstance, and the per-event metadata.creationTimestamp/
	// namespace/selfLink — pure bookkeeping that issue #411 calls out.
	extraExcluded := extraExcludedForResourceType(resourceType)

	// Pagination parameters with sensible defaults
	var limit int64 = 20 // Default page size
	if limitVal, ok := args["limit"]; ok {
		if limitFloat, ok := limitVal.(float64); ok {
			limit = int64(limitFloat)
		}
	}
	continueToken, _ := args["continue"].(string)

	opts := k8s.ListOptions{
		LabelSelector: labelSelector,
		FieldSelector: fieldSelector,
		AllNamespaces: allNamespaces,
		Limit:         limit,
		Continue:      continueToken,
	}

	// Track namespace for metrics (use "all" for cluster-wide operations)
	metricsNamespace := namespace
	if allNamespaces {
		namespace = ""
		metricsNamespace = "all"
	}

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()
	slog.Debug("acquired cluster client", slog.Duration("elapsed", time.Since(handlerStart)))

	k8sStart := time.Now()
	paginatedResponse, err := k8sClient.List(ctx, kubeContext, namespace, resourceType, apiGroup, opts)
	k8sDuration := time.Since(k8sStart)

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationList, resourceType, metricsNamespace, instrumentation.StatusError, k8sDuration)
		slog.Debug("K8s list failed",
			slog.String("resourceType", resourceType),
			slog.Duration("duration", k8sDuration),
			logging.SanitizedErr(err))
		return mcp.NewToolResultError(tools.FormatK8sError("Failed to list resources", err, client.User())), nil
	}
	slog.Debug("K8s list completed",
		slog.String("resourceType", resourceType),
		slog.Int("items", paginatedResponse.TotalItems),
		slog.Duration("duration", k8sDuration))

	// Apply client-side filtering if criteria provided
	if len(filterCriteria) > 0 {
		filteredItems, err := ApplyClientSideFilter(paginatedResponse.Items, filterCriteria)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid filter criteria: %v", err)), nil
		}
		paginatedResponse.Items = filteredItems
		paginatedResponse.TotalItems = len(paginatedResponse.Items)

		// Warn on large result sets after filtering (potential performance issue)
		if paginatedResponse.TotalItems > 1000 {
			slog.Warn("large result set after client-side filtering",
				logging.ResourceType(resourceType),
				slog.Int("item_count", paginatedResponse.TotalItems),
				slog.Int("filter_count", len(filterCriteria)))
		}
	}
	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationList, resourceType, metricsNamespace, instrumentation.StatusSuccess, k8sDuration)

	// Build output processor honouring the per-call format. SlimOutput is
	// flipped off for output=wide; secret masking always runs regardless of
	// format so the documented contract holds across every read tool.
	processor := getOutputProcessorForFormat(sc, outputFormat)
	if len(extraExcluded) > 0 && processor.Config().SlimOutput {
		processor = processorWithExtraExcluded(processor, extraExcluded)
	}

	// Handle summary mode - return aggregated counts instead of full items
	if summaryMode {
		return handleSummaryResponse(paginatedResponse.Items, processor, resourceType)
	}

	// Always run items through the processor: slim/normal apply field
	// stripping, wide skips it, but every format applies secret masking and
	// the MaxItems safety cap.
	processedItems, result, err := output.ProcessRuntimeObjects(processor, paginatedResponse.Items)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to process resources: %v", err)), nil
	}
	paginatedResponse.Items = processedItems

	// Per-resourceType post-compaction. Used for value-conditional cleanup
	// that the path-based ExcludedFields mechanism cannot express — e.g.
	// dropping eventTime / firstTimestamp / lastTimestamp on Events when
	// they are null (every event populates exactly one of the three; the
	// other two are dead bytes).
	if processor.Config().SlimOutput {
		compactItemsForResourceType(paginatedResponse.Items, resourceType)
	}

	if result.Metadata.Truncated {
		slog.Info("response truncated",
			logging.ResourceType(resourceType),
			slog.Int("final_count", result.Metadata.FinalCount),
			slog.Int("original_count", result.Metadata.OriginalCount))
	}

	if fullOutput {
		// Return full paginated output with any processing warnings
		jsonData, err := json.MarshalIndent(paginatedResponse, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal paginated resources: %v", err)), nil
		}
		slog.Debug("list resources handler completed",
			slog.Int("bytes", len(jsonData)),
			slog.Duration("elapsed", time.Since(handlerStart)))
		return mcp.NewToolResultText(string(jsonData)), nil
	}

	// Return summarized paginated output
	summary := SummarizePaginatedResources(
		paginatedResponse.Items,
		includeLabels,
		includeAnnotations,
		paginatedResponse.Continue,
		paginatedResponse.ResourceVersion,
		paginatedResponse.RemainingItems,
	)
	jsonData, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal paginated resource summary: %v", err)), nil
	}

	slog.Debug("list resources handler completed",
		slog.Int("bytes", len(jsonData)),
		slog.Duration("elapsed", time.Since(handlerStart)))
	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleDescribeResource handles kubectl describe operations
func handleDescribeResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(args)

	kubeContext, _ := args["kubeContext"].(string)
	apiGroup, _ := args["apiGroup"].(string)

	namespace, _ := args["namespace"].(string)
	// Follow kubectl behavior: if no namespace specified, use "default".
	// For cluster-scoped resources, the Kubernetes API ignores the namespace.
	if namespace == "" {
		namespace = k8s.DefaultNamespace
	}

	resourceType, ok := args["resourceType"].(string)
	if !ok || resourceType == "" {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	name, ok := args["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	// Parse output-shaping params. The schema enforces range via mcp.Min/Max,
	// but we validate again as defense-in-depth for non-compliant clients.
	eventsLimit := DefaultEventsLimit
	if v, ok := args["eventsLimit"].(float64); ok {
		val := int(v)
		if val < 1 || val > MaxEventsLimit {
			return mcp.NewToolResultError(fmt.Sprintf("eventsLimit must be between 1 and %d", MaxEventsLimit)), nil
		}
		eventsLimit = val
	}

	// Output format mirrors kubernetes_list: slim (default) and normal go through
	// the server-configured slim processor; wide returns the full manifest.
	outputFormat, _ := args["output"].(string)

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	description, err := k8sClient.Describe(ctx, kubeContext, namespace, resourceType, apiGroup, name)
	duration := time.Since(start)

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationGet, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(tools.FormatK8sError("Failed to describe resource", err, client.User())), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationGet, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Apply output processing (slim output, secret masking)
	processor := getOutputProcessorForFormat(sc, outputFormat)
	processedResource, err := output.ProcessSingleRuntimeObject(processor, description.Resource)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to process resource: %v", err)), nil
	}

	// The convenience metadata map duplicates resource.metadata.{labels,
	// annotations,uid,resourceVersion,creationTimestamp,kind,apiVersion}; run
	// it through the slim processor so the duplicate uid / resourceVersion /
	// creationTimestamp / managedFields / last-applied-configuration do not
	// show up unstripped beside the slim resource.
	processedMetadata := description.Metadata
	if outputFormat != "wide" && processor.Config().SlimOutput {
		processedMetadata = slimMetadataMap(description.Metadata, processor.Config().ExcludedFields)
	}

	result := buildDescribeOutput(processedResource, processedMetadata, description.Meta, description.Events, eventsLimit)

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal description: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// buildDescribeOutput shapes the raw describe response into a DescribeOutput,
// sorting events newest-first, truncating to eventsLimit, and stripping
// metadata.managedFields from each returned event. TotalEvents reflects the
// full pre-truncation count so callers can detect that the history was clipped.
func buildDescribeOutput(
	resource any,
	metadata map[string]any,
	meta *k8s.ResponseMeta,
	events []corev1.Event,
	limit int,
) DescribeOutput {
	out := DescribeOutput{
		Resource:    resource,
		Metadata:    metadata,
		Meta:        meta,
		TotalEvents: len(events),
		Events:      []map[string]any{},
	}

	if len(events) == 0 {
		return out
	}

	sorted := make([]corev1.Event, len(events))
	copy(sorted, events)
	sort.SliceStable(sorted, func(i, j int) bool {
		return effectiveEventTime(sorted[i]).After(effectiveEventTime(sorted[j]))
	})

	end := len(sorted)
	if limit < end {
		end = limit
	}

	slimmed := make([]map[string]any, 0, end)
	for i := range sorted[:end] {
		raw, err := json.Marshal(&sorted[i])
		if err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		slimmed = append(slimmed, output.SlimResource(m, eventSlimFields))
	}
	out.Events = slimmed
	out.ReturnedEvents = len(slimmed)
	// EventsTruncated reflects any reduction (eventsLimit or marshal failure),
	// so callers cannot read EventsTruncated=false while ReturnedEvents<TotalEvents.
	out.EventsTruncated = out.TotalEvents > out.ReturnedEvents
	return out
}

// eventSlimFields lists the per-event paths stripped from the describe
// response. The defaults below were tuned against live workloads: each
// event previously carried ~340 bytes of bookkeeping (uid, resourceVersion,
// the event's own metadata.creationTimestamp, the involvedObject duplication
// of uid/apiVersion/resourceVersion, an always-empty reportingInstance).
// Keeping them out shrinks describe responses for busy controllers by
// several KB without losing any diagnostic signal —
// firstTimestamp/lastTimestamp/eventTime/count/reason/message/
// involvedObject.{kind,name,namespace} all stay.
//
// Note: eventTime is intentionally NOT stripped. Newer event reporters
// (kyverno-scan, controller-runtime) populate eventTime instead of
// firstTimestamp/lastTimestamp, so removing it would leave those events
// without any timestamp at all. The 30-byte cost per event is worth it.
//
// The list is invariant, so it lives at package scope rather than being
// re-allocated on every describe call. SlimResource never mutates its
// excludedFields argument, so sharing the slice across calls is safe.
var eventSlimFields = []string{
	"metadata.managedFields",
	"metadata.uid",
	"metadata.resourceVersion",
	"metadata.creationTimestamp",
	"metadata.namespace",
	"metadata.selfLink",
	"involvedObject.uid",
	"involvedObject.resourceVersion",
	"involvedObject.apiVersion",
	"reportingInstance",
}

// effectiveEventTime returns the best available timestamp for sorting:
// LastTimestamp, then EventTime, then FirstTimestamp.
func effectiveEventTime(ev corev1.Event) time.Time {
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.Time
	}
	if !ev.EventTime.IsZero() {
		return ev.EventTime.Time
	}
	return ev.FirstTimestamp.Time
}

// handleCreateResource handles kubectl create operations
func handleCreateResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	if result := checkMutatingOperation(sc, "create"); result != nil {
		return result, nil
	}

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(request.GetArguments())

	kubeContext := request.GetString("kubeContext", "")
	namespace, err := request.RequireString("namespace")
	if err != nil {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	manifestData, ok := request.GetArguments()["manifest"]
	if !ok || manifestData == nil {
		return mcp.NewToolResultError("manifest is required"), nil
	}

	// Convert the manifest to a runtime.Object
	manifestJSON, err := json.Marshal(manifestData)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal manifest: %v", err)), nil
	}

	var obj runtime.Object
	if err := json.Unmarshal(manifestJSON, &obj); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse manifest: %v", err)), nil
	}

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	createdObj, err := k8sClient.Create(ctx, kubeContext, namespace, obj)
	duration := time.Since(start)

	// Extract resource type from manifest for metrics (use "unknown" if not available)
	resourceType := "unknown"
	if m, ok := manifestData.(map[string]interface{}); ok {
		if kind, ok := m["kind"].(string); ok {
			resourceType = kind
		}
	}

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationCreate, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(tools.FormatK8sError("Failed to create resource", err, client.User())), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationCreate, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Convert the created resource to JSON for output
	jsonData, err := json.MarshalIndent(createdObj, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal created resource: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleApplyResource handles kubectl apply operations
func handleApplyResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	if result := checkMutatingOperation(sc, "apply"); result != nil {
		return result, nil
	}

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(request.GetArguments())

	kubeContext := request.GetString("kubeContext", "")
	namespace, err := request.RequireString("namespace")
	if err != nil {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	manifestData, ok := request.GetArguments()["manifest"]
	if !ok || manifestData == nil {
		return mcp.NewToolResultError("manifest is required"), nil
	}

	// Convert the manifest to a runtime.Object
	manifestJSON, err := json.Marshal(manifestData)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal manifest: %v", err)), nil
	}

	var obj runtime.Object
	if err := json.Unmarshal(manifestJSON, &obj); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse manifest: %v", err)), nil
	}

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	appliedObj, err := k8sClient.Apply(ctx, kubeContext, namespace, obj)
	duration := time.Since(start)

	// Extract resource type from manifest for metrics (use "unknown" if not available)
	resourceType := "unknown"
	if m, ok := manifestData.(map[string]interface{}); ok {
		if kind, ok := m["kind"].(string); ok {
			resourceType = kind
		}
	}

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationApply, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(tools.FormatK8sError("Failed to apply resource", err, client.User())), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationApply, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Convert the applied resource to JSON for output
	jsonData, err := json.MarshalIndent(appliedObj, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal applied resource: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleDeleteResource handles kubectl delete operations
func handleDeleteResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	if result := checkMutatingOperation(sc, "delete"); result != nil {
		return result, nil
	}

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(request.GetArguments())

	kubeContext := request.GetString("kubeContext", "")
	apiGroup := request.GetString("apiGroup", "")
	namespace := request.GetString("namespace", "")
	// Follow kubectl behavior: if no namespace specified, use "default".
	// For cluster-scoped resources, the Kubernetes API ignores the namespace.
	if namespace == "" {
		namespace = k8s.DefaultNamespace
	}

	resourceType, err := request.RequireString("resourceType")
	if err != nil {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError("name is required"), nil
	}

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	deleteResponse, err := k8sClient.Delete(ctx, kubeContext, namespace, resourceType, apiGroup, name)
	duration := time.Since(start)

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationDelete, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(tools.FormatK8sError("Failed to delete resource", err, client.User())), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationDelete, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Convert the response to JSON for output (includes _meta)
	jsonData, err := json.MarshalIndent(deleteResponse, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handlePatchResource handles kubectl patch operations
func handlePatchResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	if result := checkMutatingOperation(sc, "patch"); result != nil {
		return result, nil
	}

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(request.GetArguments())

	kubeContext := request.GetString("kubeContext", "")
	apiGroup := request.GetString("apiGroup", "")
	namespace := request.GetString("namespace", "")
	// Follow kubectl behavior: if no namespace specified, use "default".
	// For cluster-scoped resources, the Kubernetes API ignores the namespace.
	if namespace == "" {
		namespace = k8s.DefaultNamespace
	}

	resourceType, err := request.RequireString("resourceType")
	if err != nil {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError("name is required"), nil
	}

	patchTypeStr, err := request.RequireString("patchType")
	if err != nil {
		return mcp.NewToolResultError("patchType is required"), nil
	}

	patchData, ok := request.GetArguments()["patch"]
	if !ok || patchData == nil {
		return mcp.NewToolResultError("patch is required"), nil
	}

	// Convert patch type string to types.PatchType
	var patchType types.PatchType
	switch patchTypeStr {
	case "strategic":
		patchType = types.StrategicMergePatchType
	case "merge":
		patchType = types.MergePatchType
	case "json":
		patchType = types.JSONPatchType
	default:
		return mcp.NewToolResultError("Invalid patch type. Must be one of: strategic, merge, json"), nil
	}

	// Convert patch data to JSON bytes
	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal patch data: %v", err)), nil
	}

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	patchResponse, err := k8sClient.Patch(ctx, kubeContext, namespace, resourceType, apiGroup, name, patchType, patchBytes)
	duration := time.Since(start)

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationPatch, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(tools.FormatK8sError("Failed to patch resource", err, client.User())), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationPatch, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Apply output processing (slim output, secret masking)
	processor := getOutputProcessor(sc)
	processedObj, err := output.ProcessSingleRuntimeObject(processor, patchResponse.Resource)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to process resource: %v", err)), nil
	}

	// Build response with metadata
	response := map[string]interface{}{
		"resource": processedObj,
		"_meta":    patchResponse.Meta,
	}

	// Convert the response to JSON for output
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal patched resource: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleScaleResource handles kubectl scale operations
func handleScaleResource(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	if result := checkMutatingOperation(sc, "scale"); result != nil {
		return result, nil
	}

	// Extract cluster parameter for multi-cluster support
	clusterName := tools.ExtractClusterParam(request.GetArguments())

	kubeContext := request.GetString("kubeContext", "")
	apiGroup := request.GetString("apiGroup", "")
	namespace, err := request.RequireString("namespace")
	if err != nil {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	resourceType, err := request.RequireString("resourceType")
	if err != nil {
		return mcp.NewToolResultError("resourceType is required"), nil
	}

	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError("name is required"), nil
	}

	replicas, err := request.RequireFloat("replicas")
	if err != nil {
		return mcp.NewToolResultError("replicas is required"), nil
	}

	// Get the appropriate k8s client (local or federated)
	client, errMsg := tools.GetClusterClient(ctx, sc, clusterName)
	if errMsg != "" {
		return mcp.NewToolResultError(errMsg), nil
	}
	k8sClient := client.K8s()

	start := time.Now()
	scaleResponse, err := k8sClient.Scale(ctx, kubeContext, namespace, resourceType, apiGroup, name, int32(replicas))
	duration := time.Since(start)

	if err != nil {
		recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationScale, resourceType, namespace, instrumentation.StatusError, duration)
		return mcp.NewToolResultError(tools.FormatK8sError("Failed to scale resource", err, client.User())), nil
	}

	recordK8sOperation(ctx, sc, clusterName, instrumentation.OperationScale, resourceType, namespace, instrumentation.StatusSuccess, duration)

	// Convert the scale response to JSON for output (includes _meta)
	jsonData, err := json.MarshalIndent(scaleResponse, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleSummaryResponse generates a summary response for large result sets.
// This provides aggregated counts by status, namespace, etc. instead of full items.
func handleSummaryResponse(items []runtime.Object, processor *output.Processor, resourceType string) (*mcp.CallToolResult, error) {
	// Convert to maps for summary generation
	maps, err := output.FromRuntimeObjects(items)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to process resources for summary: %v", err)), nil
	}

	// Generate summary
	opts := output.DefaultSummaryOptions()
	opts.IncludeByNamespace = true
	opts.IncludeByStatus = true
	opts.MaxSampleSize = 10

	summary := processor.GenerateSummary(maps, opts)

	// Build response with summary
	response := map[string]interface{}{
		"kind":           resourceType + "Summary",
		"total":          summary.Total,
		"sample":         summary.Sample,
		"hasMore":        summary.HasMore,
		"_isSummaryMode": true,
		"_hint":          "Use summary=false or add filters to see full resource details",
	}

	if len(summary.ByStatus) > 0 {
		response["byStatus"] = summary.ByStatus
	}

	if len(summary.ByNamespace) > 0 {
		// Limit namespace count to top 10 for readability
		if len(summary.ByNamespace) > 10 {
			topNamespaces := output.TopCounts(summary.ByNamespace, 10)
			nsMap := make(map[string]int)
			for _, entry := range topNamespaces {
				nsMap[entry.Key] = entry.Count
			}
			response["byNamespace"] = nsMap
			response["_namespacesTruncated"] = true
		} else {
			response["byNamespace"] = summary.ByNamespace
		}
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal summary: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}
