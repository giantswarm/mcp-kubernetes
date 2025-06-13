package helm

import (
	"context"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// RegisterHelmTools registers all Helm management tools with the MCP server
func RegisterHelmTools(s *mcpserver.MCPServer, sc *server.ServerContext) error {
	// helm_install tool
	installTool := mcp.NewTool("helm_install",
		mcp.WithDescription("Install a Helm chart"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("releaseName",
			mcp.Required(),
			mcp.Description("Name for the Helm release"),
		),
		mcp.WithString("chart",
			mcp.Required(),
			mcp.Description("Chart reference (repo/chart or path to chart)"),
		),
		mcp.WithString("namespace",
			mcp.Description("Namespace to install the chart in (optional, uses default namespace)"),
		),
		mcp.WithString("version",
			mcp.Description("Chart version to install (optional, uses latest)"),
		),
		mcp.WithObject("values",
			mcp.Description("Values to override in the chart (optional)"),
		),
		mcp.WithString("valuesFile",
			mcp.Description("Path to a values file (optional)"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Timeout in seconds for the install operation (default: 300)"),
		),
		mcp.WithBoolean("wait",
			mcp.Description("Wait for all resources to be ready (default: false)"),
		),
		mcp.WithBoolean("createNamespace",
			mcp.Description("Create the namespace if it doesn't exist (default: false)"),
		),
	)

	s.AddTool(installTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleHelmInstall(ctx, request, sc)
	})

	// helm_upgrade tool
	upgradeTool := mcp.NewTool("helm_upgrade",
		mcp.WithDescription("Upgrade a Helm release"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("releaseName",
			mcp.Required(),
			mcp.Description("Name of the Helm release to upgrade"),
		),
		mcp.WithString("chart",
			mcp.Required(),
			mcp.Description("Chart reference (repo/chart or path to chart)"),
		),
		mcp.WithString("namespace",
			mcp.Description("Namespace where the release is installed (optional)"),
		),
		mcp.WithString("version",
			mcp.Description("Chart version to upgrade to (optional, uses latest)"),
		),
		mcp.WithObject("values",
			mcp.Description("Values to override in the chart (optional)"),
		),
		mcp.WithString("valuesFile",
			mcp.Description("Path to a values file (optional)"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Timeout in seconds for the upgrade operation (default: 300)"),
		),
		mcp.WithBoolean("wait",
			mcp.Description("Wait for all resources to be ready (default: false)"),
		),
	)

	s.AddTool(upgradeTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleHelmUpgrade(ctx, request, sc)
	})

	// helm_uninstall tool
	uninstallTool := mcp.NewTool("helm_uninstall",
		mcp.WithDescription("Uninstall a Helm release"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("releaseName",
			mcp.Required(),
			mcp.Description("Name of the Helm release to uninstall"),
		),
		mcp.WithString("namespace",
			mcp.Description("Namespace where the release is installed (optional)"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Timeout in seconds for the uninstall operation (default: 300)"),
		),
		mcp.WithBoolean("wait",
			mcp.Description("Wait for all resources to be deleted (default: false)"),
		),
	)

	s.AddTool(uninstallTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleHelmUninstall(ctx, request, sc)
	})

	// helm_list tool
	listTool := mcp.NewTool("helm_list",
		mcp.WithDescription("List Helm releases"),
		mcp.WithString("kubeContext",
			mcp.Description("Kubernetes context to use (optional, uses current context if not specified)"),
		),
		mcp.WithString("namespace",
			mcp.Description("Namespace to list releases from (optional, lists from all namespaces)"),
		),
		mcp.WithBoolean("allNamespaces",
			mcp.Description("List releases from all namespaces (default: false)"),
		),
		mcp.WithString("filter",
			mcp.Description("Filter releases by name pattern (optional)"),
		),
	)

	s.AddTool(listTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleHelmList(ctx, request, sc)
	})

	return nil
}
