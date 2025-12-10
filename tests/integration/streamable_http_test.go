// Package integration provides end-to-end integration tests for mcp-kubernetes.
//
// These tests start a real MCP server and make requests to it using the mcp-go client.
// They help diagnose issues that might not be caught by unit tests.
//
// Run with: go test -v ./tests/integration/... -tags=integration
//
//go:build integration

package integration

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamableHTTPBasic tests the basic streamable-http transport functionality
// without OAuth, using a minimal MCP server setup.
func TestStreamableHTTPBasic(t *testing.T) {
	// Create a minimal MCP server for testing
	server := mcpserver.NewMCPServer(
		"test-server",
		"1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	// Add a simple test tool
	testTool := mcp.NewTool("test_echo",
		mcp.WithDescription("Echo the input message"),
		mcp.WithString("message",
			mcp.Required(),
			mcp.Description("Message to echo"),
		),
	)

	server.AddTool(testTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		message, _ := args["message"].(string)
		slog.Info("test_echo called", slog.String("message", message))
		return mcp.NewToolResultText(fmt.Sprintf("Echo: %s", message)), nil
	})

	// Create streamable HTTP handler
	httpHandler := mcpserver.NewStreamableHTTPServer(server,
		mcpserver.WithEndpointPath("/mcp"),
	)

	// Start test server
	ts := httptest.NewServer(httpHandler)
	defer ts.Close()

	t.Logf("Test server started at %s", ts.URL)

	// Create client
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mcpClient, err := client.NewStreamableHttpClient(ts.URL + "/mcp")
	require.NoError(t, err, "Failed to create MCP client")

	// Start the transport
	err = mcpClient.Start(ctx)
	require.NoError(t, err, "Failed to start MCP client transport")
	defer mcpClient.Close()

	// Initialize the client (required before other operations)
	initResult, err := mcpClient.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "integration-test",
				Version: "1.0.0",
			},
		},
	})
	require.NoError(t, err, "Failed to initialize MCP client")
	t.Logf("Server info: %s %s", initResult.ServerInfo.Name, initResult.ServerInfo.Version)

	// Give it a moment to fully initialize
	time.Sleep(100 * time.Millisecond)

	// Test: List tools (without subtest to avoid goroutine issues)
	t.Log("=== Testing ListTools ===")
	toolsResp, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Errorf("Failed to list tools: %v", err)
	} else {
		t.Logf("Found %d tools", len(toolsResp.Tools))
		for _, tool := range toolsResp.Tools {
			t.Logf("  - %s: %s", tool.Name, tool.Description)
		}
		assert.GreaterOrEqual(t, len(toolsResp.Tools), 1)
	}

	// Test: Call tool
	t.Log("=== Testing CallTool ===")
	result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Request: mcp.Request{
			Method: "tools/call",
		},
		Params: mcp.CallToolParams{
			Name: "test_echo",
			Arguments: map[string]interface{}{
				"message": "Hello, World!",
			},
		},
	})
	if err != nil {
		t.Errorf("Failed to call tool: %v", err)
	} else {
		t.Logf("Tool result: %+v", result)
		assert.NotNil(t, result)
		assert.NotEmpty(t, result.Content)
	}

	// Test: Call tool multiple times
	t.Log("=== Testing CallTool Multiple ===")
	for i := 0; i < 3; i++ {
		result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
			Request: mcp.Request{
				Method: "tools/call",
			},
			Params: mcp.CallToolParams{
				Name: "test_echo",
				Arguments: map[string]interface{}{
					"message": fmt.Sprintf("Message %d", i),
				},
			},
		})
		if err != nil {
			t.Errorf("Failed to call tool on iteration %d: %v", i, err)
		} else {
			assert.NotNil(t, result)
			t.Logf("Iteration %d result: %+v", i, result.Content)
		}
	}
}

// TestStreamableHTTPWithOAuth tests streamable-http with OAuth middleware.
// This simulates the production setup with OAuth token validation.
func TestStreamableHTTPWithOAuth(t *testing.T) {
	t.Skip("OAuth integration test requires additional setup - run manually")

	// TODO: Implement OAuth integration test
	// 1. Create mock OAuth provider
	// 2. Set up OAuth middleware
	// 3. Test token validation
	// 4. Test tool calls with valid/invalid tokens
}

// TestStreamableHTTPTimeout tests that requests don't hang indefinitely.
func TestStreamableHTTPTimeout(t *testing.T) {
	// Create a server with a slow tool
	server := mcpserver.NewMCPServer(
		"test-server",
		"1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	slowTool := mcp.NewTool("slow_tool",
		mcp.WithDescription("A slow tool that takes time"),
		mcp.WithNumber("delay_seconds",
			mcp.Description("How long to delay"),
		),
	)

	server.AddTool(slowTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		delay := 5.0 // default 5 seconds
		if d, ok := args["delay_seconds"].(float64); ok {
			delay = d
		}

		slog.Info("slow_tool sleeping", slog.Float64("delay", delay))

		select {
		case <-time.After(time.Duration(delay) * time.Second):
			return mcp.NewToolResultText("Done after delay"), nil
		case <-ctx.Done():
			return mcp.NewToolResultError("cancelled"), ctx.Err()
		}
	})

	httpHandler := mcpserver.NewStreamableHTTPServer(server,
		mcpserver.WithEndpointPath("/mcp"),
	)

	ts := httptest.NewServer(httpHandler)
	defer ts.Close()

	t.Run("TimeoutHandling", func(t *testing.T) {
		// First, initialize the client with a longer timeout
		initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer initCancel()

		mcpClient, err := client.NewStreamableHttpClient(ts.URL + "/mcp")
		require.NoError(t, err)

		err = mcpClient.Start(initCtx)
		require.NoError(t, err, "Transport start should succeed")
		defer mcpClient.Close()

		// Initialize the client
		_, err = mcpClient.Initialize(initCtx, mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				Capabilities:    mcp.ClientCapabilities{},
				ClientInfo: mcp.Implementation{
					Name:    "timeout-test",
					Version: "1.0.0",
				},
			},
		})
		require.NoError(t, err, "Client initialization should succeed")

		// Now use a short timeout for the actual tool call
		callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer callCancel()

		// Call slow tool with 10 second delay, but our context has 2 second timeout
		result, err := mcpClient.CallTool(callCtx, mcp.CallToolRequest{
			Request: mcp.Request{
				Method: "tools/call",
			},
			Params: mcp.CallToolParams{
				Name: "slow_tool",
				Arguments: map[string]interface{}{
					"delay_seconds": 10.0,
				},
			},
		})

		// Should timeout
		if err != nil {
			t.Logf("Got expected timeout error: %v", err)
			assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") ||
				strings.Contains(err.Error(), "timeout") ||
				strings.Contains(err.Error(), "canceled"),
				"Expected timeout-related error, got: %v", err)
		} else {
			t.Logf("Unexpected success: %+v", result)
			t.Fail()
		}
	})
}

// TestMain sets up logging for integration tests
func TestMain(m *testing.M) {
	// Set up structured logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	os.Exit(m.Run())
}
