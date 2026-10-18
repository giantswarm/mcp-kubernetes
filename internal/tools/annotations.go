package tools

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// ReadOnly annotates a tool that changes nothing. mcp-go defaults every tool
// to destructive and not idempotent.
func ReadOnly() mcp.ToolOption {
	return func(t *mcp.Tool) {
		mcp.WithReadOnlyHintAnnotation(true)(t)
		mcp.WithDestructiveHintAnnotation(false)(t)
		mcp.WithIdempotentHintAnnotation(true)(t)
	}
}
