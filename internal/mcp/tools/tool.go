// Package tools defines the registration boundary for future MCP tools. Phase
// 1 intentionally provides no tool implementations.
package tools

import sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

// Tool describes and registers one bounded MCP tool with the official SDK.
// Implementations remain responsible for their typed schemas and handlers.
type Tool interface {
	Name() string
	Description() string
	Register(*sdkmcp.Server) error
}
