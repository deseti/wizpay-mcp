package mcp

import (
	"fmt"
	"log/slog"
	"net/http"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewStreamableHTTPHandler binds the initialized MCP server to the official
// SDK's Streamable HTTP transport. It is stateless and registers no routes.
func NewStreamableHTTPHandler(server *Server, logger *slog.Logger) (http.Handler, error) {
	if server == nil || server.SDK() == nil {
		return nil, fmt.Errorf("initialized MCP server is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("MCP transport logger is required")
	}

	handler := sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return server.SDK() },
		&sdkmcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
			Logger:       logger,
		},
	)

	return handler, nil
}
