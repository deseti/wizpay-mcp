// Package authdiagnostics records bounded authentication transport outcomes.
// It never inspects request/response bodies, credentials, or query parameters.
package authdiagnostics

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

func route(path string) string {
	switch path {
	case "/oauth/authorize":
		return "oauth_authorize"
	case "/oauth/token":
		return "oauth_token"
	case "/browser/session":
		return "browser_session"
	case "/browser/siwe/challenge":
		return "siwe_challenge"
	case "/browser/siwe/verify":
		return "siwe_verify"
	case "/browser/consent":
		return "browser_consent"
	case "/browser/logout":
		return "browser_logout"
	}
	return ""
}

type response struct {
	http.ResponseWriter
	status int
}

// Preserve ResponseController support (particularly token/SIWE read deadlines).
func (w *response) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *response) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	if status >= 200 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *response) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func category(ctx context.Context, status int) string {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "timeout"
	case errors.Is(ctx.Err(), context.Canceled):
		return "interrupted"
	case status == http.StatusUnauthorized:
		return "session_unavailable"
	case status == http.StatusTooManyRequests:
		return "rate_limited"
	case status >= 500:
		return "server_unavailable"
	case status >= 400:
		return "http_rejected"
	default:
		return "complete"
	}
}

// Wrap emits one outcome per allowlisted request, excluding MCP streams,
// discovery, health checks and arbitrary paths. No client-supplied IDs are used.
func Wrap(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := route(r.URL.Path)
		if name == "" {
			next.ServeHTTP(w, r)
			return
		}
		id, start := rand.Text(), time.Now()
		out := &response{ResponseWriter: w}
		w.Header().Set("X-Request-ID", id)
		finished := false
		defer func() {
			status := out.status
			if status == 0 && finished {
				status = http.StatusOK
			}
			// Zero means no headers were written before a handler interruption;
			// do not manufacture a successful HTTP outcome for a panic.
			reason := category(r.Context(), status)
			if !finished {
				reason = "handler_interrupted"
			}
			logger.Info("authentication_request", "request_id", id, "route", name,
				"status", status, "duration_ms", time.Since(start).Milliseconds(), "category", reason)
		}()
		next.ServeHTTP(out, r)
		finished = true
	})
}
