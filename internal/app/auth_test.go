package app

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/config"
	"github.com/deseti/wizpay-mcp/internal/services"
)

func TestAuthenticatedServerProtectsMCPButNotHealth(t *testing.T) {
	cfg := config.Config{AppEnv: "test", ServerPort: 8080, LogLevel: "info", Auth: config.AuthConfig{Required: true, Issuer: "issuer", Audience: "audience", PublicKeyFile: "key.pem"}}
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	reached := false
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Authorization") != "Bearer valid" {
				response.WriteHeader(http.StatusUnauthorized)
				return
			}
			reached = true
			next.ServeHTTP(response, request)
		})
	}
	server, err := NewAuthenticatedServer(cfg, logger, nil, middleware)
	if err != nil {
		t.Fatal(err)
	}

	missing := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("MCP status = %d", missing.Code)
	}
	health := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d", health.Code)
	}
	valid := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	valid.Header.Set("Authorization", "Bearer valid")
	server.httpServer.Handler.ServeHTTP(httptest.NewRecorder(), valid)
	if !reached {
		t.Fatal("valid authentication did not reach MCP boundary")
	}
}

func TestAuthenticatedServerFailsClosedWithoutMiddleware(t *testing.T) {
	cfg := config.Config{AppEnv: "test", ServerPort: 8080, LogLevel: "info", Auth: config.AuthConfig{Required: true, Issuer: "issuer", Audience: "audience", PublicKeyFile: "key.pem"}}
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	if _, err := NewServer(cfg, logger); err == nil {
		t.Fatal("required authentication started without middleware")
	}
}

func TestAuthenticatedServerProtectsApprovalAPI(t *testing.T) {
	cfg := config.Config{AppEnv: "test", ServerPort: 8080, LogLevel: "info", Auth: config.AuthConfig{Required: true, Issuer: "issuer", Audience: "audience", PublicKeyFile: "key.pem"}}
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Authorization") != "Bearer valid" {
				response.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(response, request)
		})
	}
	server, err := NewAuthenticatedServerWithApproval(cfg, logger, nil, middleware, &services.PersistedApprovalService{})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/approval/apr_1", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("approval API status = %d", response.Code)
	}
}
