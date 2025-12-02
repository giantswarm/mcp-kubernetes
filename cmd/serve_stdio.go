package cmd

import (
	"fmt"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

// runStdioServer runs the server with STDIO transport
func runStdioServer(mcpSrv *mcpserver.MCPServer) error {
	// Start the server in a goroutine so we can handle shutdown signals
	serverDone := make(chan error, 1)
	go func() {
		defer close(serverDone)
		if err := mcpserver.ServeStdio(mcpSrv); err != nil {
			serverDone <- err
		}
	}()

	// Wait for server completion
	err := <-serverDone
	if err != nil {
		return fmt.Errorf("server stopped with error: %w", err)
	}

	// Don't print to stdout in stdio mode as it interferes with MCP communication
	return nil
}
