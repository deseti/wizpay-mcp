package mcp

import (
	"fmt"
	"log/slog"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/deseti/wizpay-mcp/internal/mcp/tools"
)

const (
	serviceName    = "wizpay-mcp"
	serviceVersion = "0.1.0"
)

// Server owns the official MCP SDK server and its registered tool names.
type Server struct {
	sdk *sdkmcp.Server
}

// NewServer initializes an empty MCP server and registers only the explicitly
// supplied tools. Phase 1 callers supply none.
func NewServer(logger *slog.Logger, registrations ...tools.Tool) (*Server, error) {
	if logger == nil {
		return nil, fmt.Errorf("MCP logger is required")
	}

	sdk := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: serviceName, Version: serviceVersion},
		&sdkmcp.ServerOptions{Logger: logger},
	)

	seen := make(map[string]struct{}, len(registrations))
	for _, registration := range registrations {
		if registration == nil {
			return nil, fmt.Errorf("MCP tool registration must not be nil")
		}

		name := strings.TrimSpace(registration.Name())
		if name == "" {
			return nil, fmt.Errorf("MCP tool name is required")
		}
		if strings.TrimSpace(registration.Description()) == "" {
			return nil, fmt.Errorf("MCP tool %q description is required", name)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate MCP tool %q", name)
		}
		if err := registration.Register(sdk); err != nil {
			return nil, fmt.Errorf("register MCP tool %q: %w", name, err)
		}
		seen[name] = struct{}{}
	}

	return &Server{sdk: sdk}, nil
}

// SDK returns the initialized official SDK server for transport binding.
func (s *Server) SDK() *sdkmcp.Server {
	if s == nil {
		return nil
	}
	return s.sdk
}
