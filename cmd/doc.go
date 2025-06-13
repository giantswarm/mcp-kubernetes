// Package cmd provides the command-line interface for mcp-kubernetes.
//
// This package implements a Cobra-based CLI with multiple subcommands:
//   - serve: Starts the MCP server (default behavior when no subcommand is provided)
//   - version: Displays the application version
//   - self-update: Updates the binary to the latest version from GitHub releases
//
// The CLI maintains backwards compatibility by running the serve command when
// no subcommand is specified, preserving the original behavior of the application.
//
// Command Structure:
//
//	mcp-kubernetes [flags]                 # Starts the MCP server (default)
//	mcp-kubernetes serve [flags]           # Explicitly starts the MCP server
//	mcp-kubernetes version                 # Shows version information
//	mcp-kubernetes self-update             # Updates to latest release
//	mcp-kubernetes help [command]          # Shows help information
//
// The serve command supports multiple transport options:
//   - stdio: Standard input/output (default) - for command-line integration
//   - sse: Server-Sent Events over HTTP - for web-based clients
//   - streamable-http: Streamable HTTP transport - for HTTP-based integration
//
// Transport Configuration Examples:
//
//	mcp-kubernetes serve --transport stdio           # Default STDIO transport
//	mcp-kubernetes serve --transport sse --http-addr :8080 --sse-endpoint /sse
//	mcp-kubernetes serve --transport streamable-http --http-addr :9000 --http-endpoint /mcp
//
// The serve command also supports configuration flags for controlling Kubernetes
// client behavior, including non-destructive mode, dry-run mode, and API
// rate limiting settings.
package cmd
