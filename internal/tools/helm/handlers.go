package helm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleHelmInstall handles helm install operations
func handleHelmInstall(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	// Check if non-destructive mode is enabled
	config := sc.Config()
	if config.NonDestructiveMode {
		// Check if helm install operations are allowed
		allowed := false
		for _, op := range config.AllowedOperations {
			if op == "helm_install" || op == "install" {
				allowed = true
				break
			}
		}
		if !allowed {
			return mcp.NewToolResultError("Helm install operations are not allowed in non-destructive mode"), nil
		}
	}

	args := request.GetArguments()
	
	kubeContext, _ := args["kubeContext"].(string)
	
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return mcp.NewToolResultError("namespace is required"), nil
	}
	
	releaseName, ok := args["releaseName"].(string)
	if !ok || releaseName == "" {
		return mcp.NewToolResultError("releaseName is required"), nil
	}
	
	chart, ok := args["chart"].(string)
	if !ok || chart == "" {
		return mcp.NewToolResultError("chart is required"), nil
	}

	repository, _ := args["repository"].(string)
	version, _ := args["version"].(string)
	wait, _ := args["wait"].(bool)
	createNamespace, _ := args["createNamespace"].(bool)
	
	var values map[string]interface{}
	if valuesInterface, ok := args["values"]; ok && valuesInterface != nil {
		values, _ = valuesInterface.(map[string]interface{})
	}
	
	var timeout time.Duration = 300 * time.Second
	if timeoutFloat, ok := args["timeout"].(float64); ok {
		timeout = time.Duration(timeoutFloat) * time.Second
	}

	opts := k8s.HelmInstallOptions{
		Values:          values,
		Wait:            wait,
		Timeout:         timeout,
		CreateNamespace: createNamespace,
		Version:         version,
		Repository:      repository,
	}

	release, err := sc.K8sClient().HelmInstall(ctx, kubeContext, namespace, releaseName, chart, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to install Helm chart: %v", err)), nil
	}

	// Convert release to JSON for output
	jsonData, err := json.MarshalIndent(release, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal release: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleHelmUpgrade handles helm upgrade operations
func handleHelmUpgrade(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	// Check if non-destructive mode is enabled
	config := sc.Config()
	if config.NonDestructiveMode {
		// Check if helm upgrade operations are allowed
		allowed := false
		for _, op := range config.AllowedOperations {
			if op == "helm_upgrade" || op == "upgrade" {
				allowed = true
				break
			}
		}
		if !allowed {
			return mcp.NewToolResultError("Helm upgrade operations are not allowed in non-destructive mode"), nil
		}
	}

	args := request.GetArguments()
	
	kubeContext, _ := args["kubeContext"].(string)
	
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return mcp.NewToolResultError("namespace is required"), nil
	}
	
	releaseName, ok := args["releaseName"].(string)
	if !ok || releaseName == "" {
		return mcp.NewToolResultError("releaseName is required"), nil
	}
	
	chart, ok := args["chart"].(string)
	if !ok || chart == "" {
		return mcp.NewToolResultError("chart is required"), nil
	}

	repository, _ := args["repository"].(string)
	version, _ := args["version"].(string)
	wait, _ := args["wait"].(bool)
	resetValues, _ := args["resetValues"].(bool)
	
	var values map[string]interface{}
	if valuesInterface, ok := args["values"]; ok && valuesInterface != nil {
		values, _ = valuesInterface.(map[string]interface{})
	}
	
	var timeout time.Duration = 300 * time.Second
	if timeoutFloat, ok := args["timeout"].(float64); ok {
		timeout = time.Duration(timeoutFloat) * time.Second
	}

	opts := k8s.HelmUpgradeOptions{
		Values:      values,
		Wait:        wait,
		Timeout:     timeout,
		Version:     version,
		Repository:  repository,
		ResetValues: resetValues,
	}

	release, err := sc.K8sClient().HelmUpgrade(ctx, kubeContext, namespace, releaseName, chart, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to upgrade Helm release: %v", err)), nil
	}

	// Convert release to JSON for output
	jsonData, err := json.MarshalIndent(release, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal release: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleHelmUninstall handles helm uninstall operations
func handleHelmUninstall(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	// Check if non-destructive mode is enabled
	config := sc.Config()
	if config.NonDestructiveMode {
		// Check if helm uninstall operations are allowed
		allowed := false
		for _, op := range config.AllowedOperations {
			if op == "helm_uninstall" || op == "uninstall" {
				allowed = true
				break
			}
		}
		if !allowed {
			return mcp.NewToolResultError("Helm uninstall operations are not allowed in non-destructive mode"), nil
		}
	}

	args := request.GetArguments()
	
	kubeContext, _ := args["kubeContext"].(string)
	
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return mcp.NewToolResultError("namespace is required"), nil
	}
	
	releaseName, ok := args["releaseName"].(string)
	if !ok || releaseName == "" {
		return mcp.NewToolResultError("releaseName is required"), nil
	}

	wait, _ := args["wait"].(bool)
	
	var timeout time.Duration = 300 * time.Second
	if timeoutFloat, ok := args["timeout"].(float64); ok {
		timeout = time.Duration(timeoutFloat) * time.Second
	}

	opts := k8s.HelmUninstallOptions{
		Wait:    wait,
		Timeout: timeout,
	}

	err := sc.K8sClient().HelmUninstall(ctx, kubeContext, namespace, releaseName, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to uninstall Helm release: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Helm release %s uninstalled successfully from namespace %s", releaseName, namespace)), nil
}

// handleHelmList handles helm list operations
func handleHelmList(ctx context.Context, request mcp.CallToolRequest, sc *server.ServerContext) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	
	kubeContext, _ := args["kubeContext"].(string)
	
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	allNamespaces, _ := args["allNamespaces"].(bool)
	filter, _ := args["filter"].(string)
	deployed, _ := args["deployed"].(bool)
	failed, _ := args["failed"].(bool)
	pending, _ := args["pending"].(bool)

	// If allNamespaces is true, use empty namespace
	if allNamespaces {
		namespace = ""
	}

	opts := k8s.HelmListOptions{
		AllNamespaces: allNamespaces,
		Filter:        filter,
		Deployed:      deployed,
		Failed:        failed,
		Pending:       pending,
	}

	releases, err := sc.K8sClient().HelmList(ctx, kubeContext, namespace, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list Helm releases: %v", err)), nil
	}

	// Convert releases to JSON for output
	jsonData, err := json.MarshalIndent(releases, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal releases: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
} 