package browser

import (
	"context"
	domain "github.com/deseti/wizpay-mcp/internal/browser"
	"github.com/deseti/wizpay-mcp/internal/http/authdiagnostics"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type oauthRepository = oauth.Repository

type testRepo struct {
	domain.Repository
	oauthRepository
	session     domain.Session
	transaction oauth.Transaction
}

func (r *testRepo) CreateBrowserSession(_ context.Context, s domain.Session) error {
	r.session = s
	return nil
}
func (r *testRepo) FindBrowserSession(_ context.Context, digest string) (domain.Session, error) {
	if r.session.Digest != digest {
		return domain.Session{}, domain.ErrDenied
	}
	return r.session, nil
}
func (r *testRepo) FindOAuthTransaction(context.Context, string) (oauth.Transaction, error) {
	return r.transaction, nil
}
func (r *testRepo) FindOAuthClient(context.Context, string) (oauth.Client, error) {
	return oauth.Client{ID: "client", Name: "Reviewed client", Type: "public", AuthMethod: "none", Status: "ACTIVE", RedirectURIs: []string{"https://client.example/callback"}, Scopes: []string{oauth.ReadScope}, Resources: []string{oauth.Resource}}, nil
}
func (r *testRepo) RevokeBrowserSession(context.Context, string, string) error {
	r.session.Revoked = true
	return nil
}
func (r *testRepo) DenyBrowserConsent(context.Context, string, string) error {
	r.session.Decision = "DENIED"
	return nil
}
func request(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = "connect.wizpay.xyz"
	return r
}

type deadlineRecorder struct{ *httptest.ResponseRecorder }

func (deadlineRecorder) SetReadDeadline(time.Time) error { return nil }
func setup(t *testing.T) (*Handler, string, string) {
	t.Helper()
	now := time.Now()
	repo := &testRepo{transaction: oauth.Transaction{ID: "tx", CreatedAt: now, ExpiresAt: now.Add(time.Minute), Request: oauth.AuthorizationRequest{ClientID: "client", RedirectURI: "https://client.example/callback", Scope: oauth.ReadScope, Resource: oauth.Resource}}}
	flow, e := oauth.NewService(repo, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	service, e := domain.NewService(repo, repo, flow, nil, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	h := NewHandler(service)
	w := httptest.NewRecorder()
	h.Prepare(w, request("GET", "/oauth/authorize", ""), repo.transaction)
	if w.Code != 303 {
		t.Fatal(w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("cookie missing")
	}
	c := cookies[0]
	if c.Name != CookieName || c.Domain != "" || c.Path != "/" || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatal("unsafe cookie")
	}
	if w.Header().Get("Location") != "/onboarding" {
		t.Fatal("credential in redirect")
	}
	view, e := service.View(context.Background(), c.Value)
	if e != nil {
		t.Fatal(e)
	}
	return h, c.Value, view.CSRF
}
func TestBrowserHTTPAuthorityBoundaries(t *testing.T) {
	h, raw, csrf := setup(t)
	for _, kind := range []string{"pending-grant", "missing-csrf", "wrong-csrf", "origin", "host", "forwarded", "impersonation", "missing-cookie", "unsupported-deadline"} {
		t.Run(kind, func(t *testing.T) {
			r := request("POST", "/browser/consent", `{"decision":"grant"}`)
			r.Header.Set("Origin", oauth.Issuer)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-CSRF-Token", csrf)
			r.AddCookie(&http.Cookie{Name: CookieName, Value: raw})
			want := 503
			switch kind {
			case "missing-csrf":
				r.Header.Del("X-CSRF-Token")
				want = 403
			case "wrong-csrf":
				r.Header.Set("X-CSRF-Token", strings.Repeat("a", 43))
				want = 403
			case "origin":
				r.Header.Set("Origin", "https://evil.example")
				want = 403
			case "host":
				r.Host = "mcp.wizpay.xyz"
				want = 400
			case "forwarded":
				r.Header.Set("X-Forwarded-Proto", "https")
				want = 400
			case "impersonation":
				r = request("POST", "/browser/consent", `{"decision":"grant","user_id":"victim","is_human":true}`)
				r.Header.Set("Origin", oauth.Issuer)
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("X-CSRF-Token", csrf)
				r.AddCookie(&http.Cookie{Name: CookieName, Value: raw})
				want = 400
			case "missing-cookie":
				r.Header.Del("Cookie")
				want = 401
			}
			w := httptest.NewRecorder()
			instrumented := authdiagnostics.Wrap(slog.New(slog.NewJSONHandler(io.Discard, nil)), h)
			if kind == "unsupported-deadline" {
				instrumented.ServeHTTP(w, r)
			} else {
				instrumented.ServeHTTP(deadlineRecorder{w}, r)
			}
			if w.Code != want {
				t.Fatalf("status=%d want=%d", w.Code, want)
			}
		})
	}
}
func TestBrowserDenyLogoutAndCookieIsolation(t *testing.T) {
	for _, path := range []string{"/browser/consent", "/browser/logout"} {
		t.Run(path, func(t *testing.T) {
			h, raw, csrf := setup(t)
			body := ""
			if path == "/browser/consent" {
				body = `{"decision":"deny"}`
			}
			r := request("POST", path, body)
			r.Header.Set("Origin", oauth.Issuer)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-CSRF-Token", csrf)
			r.AddCookie(&http.Cookie{Name: CookieName, Value: raw})
			w := httptest.NewRecorder()
			h.ServeHTTP(deadlineRecorder{w}, r)
			if w.Code != 200 {
				t.Fatal(w.Code)
			}
			if path == "/browser/logout" {
				if len(w.Result().Cookies()) != 0 {
					t.Fatal("logout must not erase a newer authorization cookie")
				}
				if _, err := h.service.View(context.Background(), raw); err == nil {
					t.Fatal("revoked cookie remains usable")
				}
			}
		})
	}
}

func TestStaleLogoutCannotRevokeNewAuthorization(t *testing.T) {
	h, oldCookie, oldCSRF := setup(t)
	// Prepare through the public handler replaces the browser's shared cookie.
	w := httptest.NewRecorder()
	h.Prepare(w, request("GET", "/oauth/authorize", ""), oauth.Transaction{ID: "tx", Request: oauth.AuthorizationRequest{ClientID: "client", RedirectURI: "https://client.example/callback", Scope: oauth.ReadScope, Resource: oauth.Resource}})
	if w.Code != 303 {
		t.Fatal("new authorization failed", w.Code)
	}
	newCookie := w.Result().Cookies()[0].Value
	for _, value := range []string{oldCookie, newCookie} {
		r := request("POST", "/browser/logout", "")
		r.Header.Set("Origin", oauth.Issuer)
		r.Header.Set("X-CSRF-Token", oldCSRF)
		r.AddCookie(&http.Cookie{Name: CookieName, Value: value})
		out := deadlineRecorder{httptest.NewRecorder()}
		h.ServeHTTP(out, r)
		if out.Code != 403 || len(out.Result().Cookies()) != 0 {
			t.Fatal("stale logout affected new authorization")
		}
	}
	if _, err := h.service.View(context.Background(), newCookie); err != nil {
		t.Fatal("new session revoked")
	}
}

func TestCompletedLogoutResponseCannotDeleteSubsequentAuthorizationCookie(t *testing.T) {
	h, raw, csrf := setup(t)
	r := request("POST", "/browser/logout", "")
	r.Header.Set("Origin", oauth.Issuer)
	r.Header.Set("X-CSRF-Token", csrf)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: raw})
	oldResponse := deadlineRecorder{httptest.NewRecorder()}
	h.ServeHTTP(oldResponse, r)
	if oldResponse.Code != 200 {
		t.Fatal("logout failed")
	}
	newResponse := httptest.NewRecorder()
	h.Prepare(newResponse, request("GET", "/oauth/authorize", ""), oauth.Transaction{ID: "tx", Request: oauth.AuthorizationRequest{ClientID: "client", RedirectURI: "https://client.example/callback", Scope: oauth.ReadScope, Resource: oauth.Resource}})
	if newResponse.Code != 303 {
		t.Fatal("new authorization failed")
	}
	// Even if delivered after Prepare's response, the old response cannot alter
	// the cookie jar. Its server-side revocation targeted only the original session.
	if len(oldResponse.Result().Cookies()) != 0 {
		t.Fatal("late response mutates shared cookie")
	}
	if _, err := h.service.View(context.Background(), newResponse.Result().Cookies()[0].Value); err != nil {
		t.Fatal("new authority unavailable")
	}
}
