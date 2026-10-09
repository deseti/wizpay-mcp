// Package browser exposes no identity-verification endpoint in WP3A.
package browser

import (
	"encoding/json"
	"errors"
	domain "github.com/deseti/wizpay-mcp/internal/browser"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"io"
	"mime"
	"net/http"
	"time"
)

const CookieName = "__Host-wizpay-browser"

type Handler struct{ service *domain.Service }

func NewHandler(s *domain.Service) *Handler { return &Handler{s} }
func safe(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
func reply(w http.ResponseWriter, status int, v any) {
	safe(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func valid(r *http.Request) bool {
	return r.Host == "connect.wizpay.xyz" && !r.URL.IsAbs() && r.Header.Get("Forwarded") == "" && r.Header.Get("X-Forwarded-Host") == "" && r.Header.Get("X-Forwarded-Proto") == ""
}
func cookie(w http.ResponseWriter, raw string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: raw, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}
func credential(r *http.Request) (string, bool) {
	value := ""
	count := 0
	for _, c := range r.Cookies() {
		if c.Name == CookieName {
			value = c.Value
			count++
		}
	}
	return value, count == 1
}

// Prepare accepts only WP2-validated durable transactions. No transaction or
// session credential appears in the onboarding URL.
func (h *Handler) Prepare(w http.ResponseWriter, r *http.Request, t oauth.Transaction) {
	safe(w)
	if !valid(r) || h.service == nil {
		reply(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	raw, e := h.service.Start(r.Context(), t)
	if e != nil {
		reply(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	cookie(w, raw, int(domain.PendingLifetime.Seconds()))
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	safe(w)
	if !valid(r) || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
		reply(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != oauth.Issuer {
		reply(w, 403, map[string]string{"error": "access_denied"})
		return
	}
	raw, ok := credential(r)
	if !ok || h.service == nil {
		reply(w, 401, map[string]string{"error": "session_unavailable"})
		return
	}
	if r.URL.Path == "/browser/session" && r.Method == "GET" {
		v, e := h.service.View(r.Context(), raw)
		if e != nil {
			reply(w, 401, map[string]string{"error": "session_expired"})
			return
		}
		reply(w, 200, v)
		return
	}
	if (r.URL.Path != "/browser/consent" && r.URL.Path != "/browser/logout" && r.URL.Path != "/browser/siwe/challenge" && r.URL.Path != "/browser/siwe/verify") || r.Method != "POST" {
		reply(w, 404, map[string]string{"error": "invalid_request"})
		return
	}
	if r.Header.Get("Origin") != oauth.Issuer || len(r.Header.Values("X-CSRF-Token")) != 1 {
		reply(w, 403, map[string]string{"error": "access_denied"})
		return
	}
	ctl := http.NewResponseController(w)
	if e := ctl.SetReadDeadline(time.Now().Add(5 * time.Second)); e != nil {
		w.Header().Set("Connection", "close")
		reply(w, 503, map[string]string{"error": "temporarily_unavailable"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	defer r.Body.Close()
	proof := r.Header.Get("X-CSRF-Token")
	if r.URL.Path == "/browser/siwe/challenge" || r.URL.Path == "/browser/siwe/verify" {
		h.walletAction(w, r, raw, proof, ctl)
		return
	}
	var err error
	if r.URL.Path == "/browser/logout" {
		// Logout carries no identity or decision input.
		body, e := io.ReadAll(r.Body)
		if e != nil || len(body) != 0 {
			w.Header().Set("Connection", "close")
			reply(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		err = h.service.Logout(r.Context(), raw, proof)
		if err == nil {
			cookie(w, "", -1)
		}
	} else {
		mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaErr != nil || mediaType != "application/json" {
			w.Header().Set("Connection", "close")
			reply(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		var body struct {
			Decision string `json:"decision"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if e := decoder.Decode(&body); e != nil {
			w.Header().Set("Connection", "close")
			reply(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		if e := decoder.Decode(new(any)); e != io.EOF {
			w.Header().Set("Connection", "close")
			reply(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		switch body.Decision {
		case "deny":
			err = h.service.Deny(r.Context(), raw, proof)
		case "grant":
			var location string
			location, err = h.service.Grant(r.Context(), raw, proof)
			if err == nil {
				_ = ctl.SetReadDeadline(time.Time{})
				reply(w, 200, map[string]string{"redirect": location})
				return
			}
		default:
			err = domain.ErrDenied
		}
	}
	_ = ctl.SetReadDeadline(time.Time{})
	if err != nil {
		status := 403
		if errors.Is(err, domain.ErrUnavailable) {
			status = 503
		}
		reply(w, status, map[string]string{"error": "authorization_unavailable"})
		return
	}
	reply(w, 200, map[string]string{"state": "complete"})
}
