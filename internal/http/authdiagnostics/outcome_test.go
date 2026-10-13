package authdiagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type deadlineWriter struct {
	*httptest.ResponseRecorder
	called bool
}

func (w *deadlineWriter) SetReadDeadline(time.Time) error { w.called = true; return nil }

func TestOutcomesContainOnlyAllowlistedMetadata(t *testing.T) {
	for _, tc := range []struct {
		status   int
		category string
	}{
		{200, "complete"}, {303, "complete"}, {400, "http_rejected"}, {401, "session_unavailable"},
		{403, "http_rejected"}, {429, "rate_limited"}, {503, "server_unavailable"},
	} {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		secret := "SYNTHETIC_AUTH_MATERIAL_MUST_NOT_BE_LOGGED"
		h := Wrap(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := http.NewResponseController(w).SetReadDeadline(time.Now()); err != nil {
				t.Fatal("deadline interface lost", err)
			}
			w.Header().Set("Set-Cookie", secret)
			w.Header().Set("Location", "https://client.example/callback?code="+secret)
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(secret))
		}))
		req := httptest.NewRequest("POST", "https://connect.wizpay.xyz/browser/siwe/verify?code="+secret, strings.NewReader(`{"signature":"`+secret+`","challenge_id":"`+secret+`","code_verifier":"`+secret+`"}`))
		for _, key := range []string{"Authorization", "Cookie", "X-CSRF-Token", "X-Request-ID", "Referer"} {
			req.Header.Set(key, secret)
		}
		out := &deadlineWriter{ResponseRecorder: httptest.NewRecorder()}
		h.ServeHTTP(out, req)
		if !out.called || out.Code != tc.status {
			t.Fatal("handler behavior changed")
		}
		if strings.Contains(logs.String(), secret) || strings.Contains(logs.String(), "client.example") {
			t.Fatal("sensitive material in logs")
		}
		var record map[string]any
		if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		allowed := map[string]bool{"time": true, "level": true, "msg": true, "request_id": true, "route": true, "status": true, "duration_ms": true, "category": true}
		if len(record) != len(allowed) {
			t.Fatal("unexpected field count")
		}
		for key := range record {
			if !allowed[key] {
				t.Fatal("unexpected diagnostic field", key)
			}
		}
		if record["route"] != "siwe_verify" || record["category"] != tc.category || record["status"] != float64(tc.status) {
			t.Fatal("incorrect outcome classification")
		}
		if id, ok := record["request_id"].(string); !ok || len(id) < 20 || id != out.Header().Get("X-Request-ID") {
			t.Fatal("missing server-generated identifier")
		}
		if duration, ok := record["duration_ms"].(float64); !ok || duration < 0 {
			t.Fatal("invalid duration")
		}
	}
}

func TestRouteAllowlistAndUniqueIdentifiers(t *testing.T) {
	seen := map[string]bool{}
	for _, path := range []string{"/oauth/authorize", "/oauth/token", "/browser/session", "/browser/siwe/challenge", "/browser/siwe/verify", "/browser/consent", "/browser/logout", "/mcp", "/health", "/.well-known/oauth-authorization-server", "/browser/private-untrusted-value"} {
		var logs bytes.Buffer
		h := Wrap(slog.New(slog.NewJSONHandler(&logs, nil)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
		out := httptest.NewRecorder()
		h.ServeHTTP(out, httptest.NewRequest("GET", path, nil))
		if route(path) == "" {
			if logs.Len() != 0 || out.Header().Get("X-Request-ID") != "" {
				t.Fatal("unrelated route instrumented")
			}
			continue
		}
		if logs.Len() == 0 {
			t.Fatal("missing authentication outcome")
		}
		id := out.Header().Get("X-Request-ID")
		if id == "" || seen[id] {
			t.Fatal("identifier reused")
		}
		seen[id] = true
	}
}

func TestCanceledRequestsUseBoundedCategories(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	timeout, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	for _, tc := range []struct {
		ctx  context.Context
		want string
	}{{ctx, "interrupted"}, {timeout, "timeout"}} {
		var logs bytes.Buffer
		h := Wrap(slog.New(slog.NewJSONHandler(&logs, nil)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/browser/session", nil).WithContext(tc.ctx))
		var record map[string]any
		if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		if record["category"] != tc.want {
			t.Fatal("incorrect cancellation category")
		}
	}
}

func TestHandlerPanicIsNotSwallowedOrLoggedAsSuccess(t *testing.T) {
	var logs bytes.Buffer
	h := Wrap(slog.New(slog.NewJSONHandler(&logs, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("synthetic-sensitive-panic") }))
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic swallowed")
			}
		}()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/browser/session", nil))
	}()
	var record map[string]any
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["status"] != float64(0) || record["category"] != "handler_interrupted" || strings.Contains(logs.String(), "synthetic-sensitive-panic") {
		t.Fatal("unsafe interrupted outcome")
	}
}
