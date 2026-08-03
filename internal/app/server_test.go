package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/config"
)

type failingReadiness struct{}

func (failingReadiness) Ping(context.Context) error { return errors.New("database unavailable") }

func newTestServer(t *testing.T) *Server {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	server, err := NewServer(config.Config{AppEnv: "test", ServerPort: 8080, LogLevel: "info"}, logger)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	return server
}

func TestHealth(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != "{\"status\":\"ok\",\"service\":\"wizpay-mcp\"}\n" {
		t.Fatalf("health response = %d %q", response.Code, response.Body.String())
	}
}

func TestReadinessTracksLifecycle(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodGet, "/readiness", nil)

	notReady := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(notReady, request)
	if notReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness before startup = %d, want %d", notReady.Code, http.StatusServiceUnavailable)
	}

	server.ready.Store(true)
	ready := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(ready, request)
	if ready.Code != http.StatusOK || ready.Body.String() != "{\"status\":\"ok\",\"service\":\"wizpay-mcp\"}\n" {
		t.Fatalf("readiness response = %d %q", ready.Code, ready.Body.String())
	}
}

func TestHealthRejectsUnsupportedMethod(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodPost, "/health", nil)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("health POST = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestReadinessFailsClosedWhenDependencyIsUnavailable(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	server, err := NewServerWithReadiness(config.Config{AppEnv: "test", ServerPort: 8080, LogLevel: "info"}, logger, failingReadiness{})
	if err != nil {
		t.Fatal(err)
	}
	server.ready.Store(true)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readiness", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness = %d", response.Code)
	}
}
