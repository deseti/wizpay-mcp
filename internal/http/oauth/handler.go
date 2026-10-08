// Package oauth serves only protocol/discovery endpoints. There is deliberately
// no browser authentication, consent completion, registration, or test-login route.
package oauth

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	domain "github.com/deseti/wizpay-mcp/internal/oauth"
)

// TokenBodyReadTimeout bounds only token body reads; MCP streams are unaffected.
const TokenBodyReadTimeout = 5 * time.Second

type Handler struct {
	service *domain.Service
	mu      sync.Mutex
	window  time.Time
	count   int
}

func NewHandler(s *domain.Service) *Handler { return &Handler{service: s} }
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func protocolError(w http.ResponseWriter, status int, code string) {
	write(w, status, map[string]string{"error": code})
}
func (h *Handler) allowed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	if now.Sub(h.window) >= time.Minute {
		h.window = now
		h.count = 0
	}
	h.count++
	return h.count <= 120
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Host is the original public authority. Forwarded headers are never trusted.
	// An edge must terminate HTTPS, preserve Host and isolate the upstream listener.
	host := r.Host
	if r.URL.IsAbs() || (r.URL.Scheme != "" && r.URL.Scheme != "https") {
		protocolError(w, 400, "invalid_request")
		return
	}
	if host == "mcp.wizpay.xyz" && (r.URL.Path == "/.well-known/oauth-protected-resource" || r.URL.Path == "/.well-known/oauth-protected-resource/mcp") {
		if r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			protocolError(w, 405, "invalid_request")
			return
		}
		write(w, 200, map[string]any{"resource": domain.Resource, "authorization_servers": []string{domain.Issuer}, "scopes_supported": []string{domain.ReadScope}, "bearer_methods_supported": []string{"header"}})
		return
	}
	if host != "connect.wizpay.xyz" {
		protocolError(w, 404, "invalid_request")
		return
	}
	if r.URL.Path == "/.well-known/oauth-authorization-server" {
		if r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			protocolError(w, 405, "invalid_request")
			return
		}
		write(w, 200, map[string]any{"issuer": domain.Issuer, "authorization_endpoint": domain.Issuer + "/oauth/authorize", "token_endpoint": domain.Issuer + "/oauth/token", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code"}, "code_challenge_methods_supported": []string{"S256"}, "scopes_supported": []string{domain.ReadScope}, "token_endpoint_auth_methods_supported": []string{"none"}, "authorization_response_iss_parameter_supported": true, "client_id_metadata_document_supported": false})
		return
	}
	if r.URL.Path != "/oauth/authorize" && r.URL.Path != "/oauth/token" {
		protocolError(w, 404, "invalid_request")
		return
	}
	// Bound token bodies even when admission or header validation rejects early.
	if r.URL.Path == "/oauth/token" {
		controller := http.NewResponseController(w)
		if err := controller.SetReadDeadline(time.Now().Add(TokenBodyReadTimeout)); err != nil {
			// Unsupported wrappers/transports must fail closed before reading a body.
			w.Header().Set("Connection", "close")
			protocolError(w, 503, "temporarily_unavailable")
			return
		}
	}
	if !h.allowed() {
		protocolError(w, 429, "temporarily_unavailable")
		return
	}
	if h.service == nil {
		protocolError(w, 503, "temporarily_unavailable")
		return
	}
	if r.URL.Path == "/oauth/authorize" {
		h.authorize(w, r)
		return
	}
	h.token(w, r)
}
func one(v url.Values, key string) string {
	if len(v[key]) != 1 {
		return ""
	}
	return v[key][0]
}
func known(v url.Values, keys ...string) bool {
	allowed := map[string]bool{}
	for _, k := range keys {
		allowed[k] = true
	}
	for k, values := range v {
		if !allowed[k] || len(values) != 1 {
			return false
		}
	}
	return true
}
func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		protocolError(w, 405, "invalid_request")
		return
	}
	if len(r.URL.RawQuery) > 4096 {
		protocolError(w, 400, "invalid_request")
		return
	}
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil || !known(q, "client_id", "redirect_uri", "resource", "scope", "response_type", "code_challenge", "code_challenge_method", "state") {
		protocolError(w, 400, "invalid_request")
		return
	}
	if q.Get("response_type") != "code" {
		protocolError(w, 400, "unsupported_response_type")
		return
	}
	if q.Get("resource") != domain.Resource {
		protocolError(w, 400, "invalid_target")
		return
	}
	if q.Get("scope") != domain.ReadScope {
		protocolError(w, 400, "invalid_scope")
		return
	}
	_, e = h.service.Begin(r.Context(), domain.AuthorizationRequest{ClientID: one(q, "client_id"), RedirectURI: one(q, "redirect_uri"), Resource: one(q, "resource"), Scope: one(q, "scope"), ResponseType: one(q, "response_type"), Challenge: one(q, "code_challenge"), ChallengeMethod: one(q, "code_challenge_method"), State: q.Get("state")})
	if e != nil {
		protocolError(w, 400, "invalid_request")
		return
	}
	// No redirect or transaction handle is disclosed before WP3 establishes a
	// browser session/CSRF-bound continuation. Anonymous durable state expires.
	write(w, 503, map[string]string{"error": "temporarily_unavailable", "error_description": "Verified browser authentication and consent are not available."})
}
func (h *Handler) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		protocolError(w, 405, "invalid_request")
		return
	}
	// Public-client profile: no browser cookies, HTTP Basic, client secrets, query
	// credentials, or cross-origin browser token exchanges. No CORS permission.
	if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" || r.URL.RawQuery != "" {
		protocolError(w, 400, "invalid_request")
		return
	}
	typ, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || typ != "application/x-www-form-urlencoded" {
		protocolError(w, 400, "invalid_request")
		return
	}
	controller := http.NewResponseController(w)
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if e = r.ParseForm(); e != nil {
		// Keep the deadline on failed reads and close HTTP/1 connections: clearing
		// it here could let net/http drain an incomplete body indefinitely.
		w.Header().Set("Connection", "close")
		protocolError(w, 400, "invalid_request")
		return
	}
	if err := controller.SetReadDeadline(time.Time{}); err != nil {
		protocolError(w, 503, "temporarily_unavailable")
		return
	}
	if !known(r.PostForm, "grant_type", "client_id", "redirect_uri", "resource", "code", "code_verifier") {
		protocolError(w, 400, "invalid_request")
		return
	}
	if one(r.PostForm, "grant_type") != "authorization_code" {
		protocolError(w, 400, "unsupported_grant_type")
		return
	}
	if one(r.PostForm, "resource") != domain.Resource {
		protocolError(w, 400, "invalid_target")
		return
	}
	x := domain.Exchange{GrantType: one(r.PostForm, "grant_type"), ClientID: one(r.PostForm, "client_id"), RedirectURI: one(r.PostForm, "redirect_uri"), Resource: one(r.PostForm, "resource"), Code: one(r.PostForm, "code"), Verifier: one(r.PostForm, "code_verifier")}
	t, e := h.service.Exchange(r.Context(), x)
	if e != nil {
		code := "invalid_grant"
		if errors.Is(e, domain.ErrInvalidClient) {
			code = "invalid_client"
		}
		protocolError(w, 400, code)
		return
	}
	write(w, 200, t)
}

// ProtectOrigin wraps protected resource routes before bearer authentication.
func ProtectOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "mcp.wizpay.xyz" || r.URL.IsAbs() {
			protocolError(w, 400, "invalid_request")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != domain.ResourceOrigin {
			protocolError(w, 403, "access_denied")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Routes muxes strictly disjoint public origins. Existing health routes remain
// upstream-private; connect never exposes MCP or approval bearer endpoints.
func Routes(protocol *Handler, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/") || strings.HasPrefix(r.URL.Path, "/oauth/") {
			protocol.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/health" || r.URL.Path == "/readiness" {
			next.ServeHTTP(w, r)
			return
		}
		ProtectOrigin(next).ServeHTTP(w, r)
	})
}
