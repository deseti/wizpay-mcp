// Package app wires the Phase 1 process dependencies and HTTP surface.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/deseti/wizpay-mcp/internal/config"
	approvalhttp "github.com/deseti/wizpay-mcp/internal/http/approval"
	internalmcp "github.com/deseti/wizpay-mcp/internal/mcp"
	"github.com/deseti/wizpay-mcp/internal/mcp/tools"
	"github.com/deseti/wizpay-mcp/internal/services"
)

const (
	serviceName       = "wizpay-mcp"
	readHeaderTimeout = 5 * time.Second
	idleTimeout       = 60 * time.Second
	maxHeaderBytes    = 1 << 20
)

// Server owns the application HTTP server and process lifecycle state.
type Server struct {
	config     config.Config
	logger     *slog.Logger
	httpServer *http.Server
	ready      atomic.Bool
	readiness  ReadinessChecker
}

type ReadinessChecker interface{ Ping(context.Context) error }

// NewServer initializes dependencies in configuration, MCP, transport, then
// HTTP routing order. It does not open a network listener.
func NewServer(cfg config.Config, logger *slog.Logger, registrations ...tools.Tool) (*Server, error) {
	return newServer(cfg, logger, nil, nil, registrations...)
}

func NewServerWithReadiness(cfg config.Config, logger *slog.Logger, readiness ReadinessChecker, registrations ...tools.Tool) (*Server, error) {
	return newServer(cfg, logger, readiness, nil, registrations...)
}

// NewAuthenticatedServer wires authentication only around the MCP endpoint.
// Health and readiness remain unauthenticated.
func NewAuthenticatedServer(cfg config.Config, logger *slog.Logger, readiness ReadinessChecker, authentication func(http.Handler) http.Handler, registrations ...tools.Tool) (*Server, error) {
	if !cfg.Auth.Required || authentication == nil {
		return nil, fmt.Errorf("required authentication middleware is missing")
	}
	return newServer(cfg, logger, readiness, authentication, registrations...)
}

// NewAuthenticatedServerWithApproval adds the authenticated approval HTTP
// surface while preserving the existing MCP and health/readiness routes.
func NewAuthenticatedServerWithApproval(cfg config.Config, logger *slog.Logger, readiness ReadinessChecker, authentication func(http.Handler) http.Handler, approvalService services.ApprovalService, registrations ...tools.Tool) (*Server, error) {
	if !cfg.Auth.Required || authentication == nil {
		return nil, fmt.Errorf("required authentication middleware is missing")
	}
	approvalHandler, err := approvalhttp.NewHandler(approvalService)
	if err != nil {
		return nil, err
	}
	return newServerWithApproval(cfg, logger, readiness, authentication, approvalHandler, registrations...)
}

func newServer(cfg config.Config, logger *slog.Logger, readiness ReadinessChecker, authentication func(http.Handler) http.Handler, registrations ...tools.Tool) (*Server, error) {
	return newServerWithApproval(cfg, logger, readiness, authentication, nil, registrations...)
}

func newServerWithApproval(cfg config.Config, logger *slog.Logger, readiness ReadinessChecker, authentication func(http.Handler) http.Handler, approvalHandler http.Handler, registrations ...tools.Tool) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate application configuration: %w", err)
	}
	if logger == nil {
		return nil, fmt.Errorf("application logger is required")
	}

	mcpServer, err := internalmcp.NewServer(logger, registrations...)
	if err != nil {
		return nil, fmt.Errorf("initialize MCP server: %w", err)
	}
	mcpHandler, err := internalmcp.NewStreamableHTTPHandler(mcpServer, logger)
	if err != nil {
		return nil, fmt.Errorf("initialize MCP transport: %w", err)
	}

	server := &Server{config: cfg, logger: logger, readiness: readiness}
	mux := http.NewServeMux()
	if cfg.Auth.Required {
		if authentication == nil {
			return nil, fmt.Errorf("required authentication middleware is missing")
		}
		mcpHandler = authentication(mcpHandler)
	}
	mux.Handle("/mcp", mcpHandler)
	if approvalHandler != nil {
		mux.Handle("/approval/", authentication(approvalHandler))
		mux.Handle("/approvals", authentication(approvalHandler))
	}
	mux.HandleFunc("/health", server.healthHandler)
	mux.HandleFunc("/readiness", server.readinessHandler)

	server.httpServer = &http.Server{
		Addr:              cfg.Address(),
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	logger.Info("configuration_loaded",
		"app_env", cfg.AppEnv,
		"server_port", cfg.ServerPort,
		"log_level", cfg.LogLevel,
	)

	return server, nil
}

// Run opens the HTTP listener and blocks until startup failure or graceful
// context cancellation.
func (s *Server) Run(ctx context.Context) error {
	if s == nil || s.httpServer == nil {
		return fmt.Errorf("initialized application server is required")
	}
	if ctx == nil {
		return fmt.Errorf("application context is required")
	}

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.httpServer.Addr, err)
	}

	s.ready.Store(true)
	s.logger.Info("server_started", "address", listener.Addr().String())
	defer s.ready.Store(false)
	readinessContext, stopReadiness := context.WithCancel(ctx)
	defer stopReadiness()
	go func() {
		<-readinessContext.Done()
		s.ready.Store(false)
	}()

	return serveUntilCanceled(ctx, s.httpServer, listener, s.logger)
}

type statusResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

func (s *Server) healthHandler(response http.ResponseWriter, request *http.Request) {
	writeStatus(response, request, http.StatusOK, "ok")
}

func (s *Server) readinessHandler(response http.ResponseWriter, request *http.Request) {
	if !s.ready.Load() {
		writeStatus(response, request, http.StatusServiceUnavailable, "not_ready")
		return
	}
	if s.readiness != nil {
		ctx, cancel := context.WithTimeout(request.Context(), time.Second)
		defer cancel()
		if err := s.readiness.Ping(ctx); err != nil {
			writeStatus(response, request, http.StatusServiceUnavailable, "not_ready")
			return
		}
	}
	writeStatus(response, request, http.StatusOK, "ok")
}

func writeStatus(response http.ResponseWriter, request *http.Request, statusCode int, status string) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		response.Header().Set("Allow", "GET, HEAD")
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(statusCode)
	if request.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(response).Encode(statusResponse{Status: status, Service: serviceName})
}
