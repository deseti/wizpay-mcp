package mcp

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type testTool struct {
	name        string
	description string
	err         error
	registered  bool
}

func (t *testTool) Name() string        { return t.name }
func (t *testTool) Description() string { return t.description }
func (t *testTool) Register(*sdkmcp.Server) error {
	t.registered = true
	return t.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

func TestNewServerInitializesEmptyServer(t *testing.T) {
	server, err := NewServer(testLogger())
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	if server.SDK() == nil {
		t.Fatal("NewServer() returned nil SDK server")
	}
}

func TestNewServerRegistersTools(t *testing.T) {
	tool := &testTool{name: "test_tool", description: "test registration boundary"}
	if _, err := NewServer(testLogger(), tool); err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	if !tool.registered {
		t.Fatal("tool was not registered")
	}
}

func TestNewServerRejectsDuplicateTools(t *testing.T) {
	first := &testTool{name: "duplicate", description: "first"}
	second := &testTool{name: "duplicate", description: "second"}
	_, err := NewServer(testLogger(), first, second)
	if err == nil || !strings.Contains(err.Error(), "duplicate MCP tool") {
		t.Fatalf("NewServer() error = %v, want duplicate error", err)
	}
}

func TestNewServerWrapsRegistrationError(t *testing.T) {
	tool := &testTool{name: "broken", description: "broken registration", err: errors.New("registration failed")}
	_, err := NewServer(testLogger(), tool)
	if err == nil || !strings.Contains(err.Error(), "register MCP tool") {
		t.Fatalf("NewServer() error = %v, want registration error", err)
	}
}

func TestNewStreamableHTTPHandler(t *testing.T) {
	server, err := NewServer(testLogger())
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	handler, err := NewStreamableHTTPHandler(server, testLogger())
	if err != nil {
		t.Fatalf("NewStreamableHTTPHandler() error = %v", err)
	}
	if handler == nil {
		t.Fatal("NewStreamableHTTPHandler() returned nil handler")
	}
}

func TestStreamableHTTPInitialization(t *testing.T) {
	server, err := NewServer(testLogger())
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	handler, err := NewStreamableHTTPHandler(server, testLogger())
	if err != nil {
		t.Fatalf("NewStreamableHTTPHandler() error = %v", err)
	}

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("initialize response = %d %q", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"name":"wizpay-mcp"`) {
		t.Fatalf("initialize response missing server identity: %s", response.Body.String())
	}
}
