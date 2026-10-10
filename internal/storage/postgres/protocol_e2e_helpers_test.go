package postgres

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/app"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/browser"
	"github.com/deseti/wizpay-mcp/internal/config"
	browserhttp "github.com/deseti/wizpay-mcp/internal/http/browser"
	"github.com/deseti/wizpay-mcp/internal/mcp/tools"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
	"github.com/deseti/wizpay-mcp/internal/services"
	"github.com/deseti/wizpay-mcp/internal/siwe"
	"github.com/deseti/wizpay-mcp/internal/wallet"
	"github.com/ethereum/go-ethereum/crypto"
)

// No socket is opened. Complete in-memory bodies use the existing handler-test
// deadline adapter; connection deadlines/TLS/browser cookie enforcement are not
// asserted by this harness. All authentication and persistence checks are real.
type protocolRecorder struct{ *httptest.ResponseRecorder }

func (protocolRecorder) SetReadDeadline(time.Time) error { return nil }

type protocolFixture struct {
	base    fixture
	client  oauth.Client
	store   *Store
	handler http.Handler
	offset  *atomic.Int64
	address string
}
type protocolLogin struct {
	pending, authenticated *http.Cookie
	pendingView, view      browser.View
	challenge              struct {
		ID        string    `json:"challenge_id"`
		Message   string    `json:"message"`
		Address   string    `json:"address"`
		ExpiresAt time.Time `json:"expires_at"`
	}
}

func newProtocolFixture(t *testing.T, chain string) *protocolFixture {
	t.Helper()
	base := createBaseFixture(t, false)
	// Explicit test-only review/activation, never automatic production onboarding.
	if _, err := integrationPool.Exec(context.Background(), `UPDATE tenants SET status='ACTIVE' WHERE tenant_id=$1`, base.scope.TenantID()); err != nil {
		t.Fatal("activate synthetic tenant:", err)
	}
	key, err := crypto.HexToECDSA(strings.Repeat("0", 63) + "1")
	if err != nil {
		t.Fatal(err)
	}
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	now := time.Now().UTC()
	binding, err := wallet.NewBinding(wallet.BindingParams{BindingID: unique("protocol-binding"), Version: 1, UserID: base.scope.ActorID(), Provider: "EXTERNAL_EVM", ProviderUserReference: "synthetic-reviewed-owner", WalletID: unique("protocol-wallet"), Address: address, ChainID: chain, Network: "MAINNET", Status: wallet.BindingStatusActive, VerificationReference: "test-only-reviewed-binding", CreatedAt: now.Add(-time.Minute), VerifiedAt: now.Add(-time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.CreateBinding(context.Background(), base.scope, binding); err != nil {
		t.Fatal(err)
	}
	client := oauth.Client{ID: unique("protocol-client"), Name: "Synthetic E2E client", Type: "public", AuthMethod: "none", Status: "ACTIVE", RedirectURIs: []string{"https://client.example/callback"}, Scopes: []string{oauth.ReadScope}, Resources: []string{oauth.Resource}}
	if err = integrationStore.RegisterOAuthClient(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	store := *integrationStore
	offset := &atomic.Int64{}
	store.now = func() time.Time { return time.Now().Add(time.Duration(offset.Load())) }
	flow, err := oauth.NewService(&store, store.now)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := siwe.NewService(&store, base.scope.TenantID(), siwe.URI)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := browser.NewWalletService(&store, &store, flow, verifier, store.now)
	if err != nil {
		t.Fatal(err)
	}
	middleware, err := requestauth.NewOAuthMiddleware(flow, requestauth.RepositoryResolver{Repository: &store})
	if err != nil {
		t.Fatal(err)
	}
	authorizer := auth.NewPermissionAuthorizer()
	intents := &services.PersistedIntentService{Intents: &store, Wallets: &store, Authorizer: authorizer, Audit: &store, Now: store.now}
	approvals := &services.PersistedApprovalService{Approvals: &store, Intents: &store, Wallets: &store, Authorizer: authorizer, Audit: &store, Now: store.now}
	registry, err := tools.NewOAuthReadOnlyRegistry(intents, approvals)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AppEnv: "test", ServerPort: 8080, LogLevel: "info", OAuthEnabled: true, SIWEEnabled: true, OnboardingTenantID: base.scope.TenantID(), SIWEOrigin: siwe.URI, Auth: config.AuthConfig{Required: true}}
	server, err := app.NewOAuthServerWithBrowserFoundation(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), &store, middleware.Wrap, approvals, flow, sessions, registry.Tools()...)
	if err != nil {
		t.Fatal(err)
	}
	return &protocolFixture{base, client, &store, server, offset, address}
}
func (f *protocolFixture) authorizeQuery() url.Values {
	return url.Values{"client_id": {f.client.ID}, "redirect_uri": {f.client.RedirectURIs[0]}, "resource": {oauth.Resource}, "scope": {oauth.ReadScope}, "response_type": {"code"}, "code_challenge": {oauth.Digest(oauthVerifier)}, "code_challenge_method": {"S256"}, "state": {"synthetic-client-state"}}
}
func (f *protocolFixture) request(host, method, path, body string, cookie *http.Cookie, csrf, bearer string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = host
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if strings.HasPrefix(path, "/browser/") && method == "POST" {
		r.Header.Set("Origin", oauth.Issuer)
	}
	if csrf != "" {
		r.Header.Set("X-CSRF-Token", csrf)
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	if path == "/oauth/token" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r.Header.Set("Content-Type", "application/json")
	}
	if path == "/mcp" {
		r.Header.Set("Accept", "application/json, text/event-stream")
	}
	return r
}
func (f *protocolFixture) send(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(protocolRecorder{w}, r)
	return w
}
func (f *protocolFixture) browser(method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	return f.send(f.request("connect.wizpay.xyz", method, path, body, cookie, csrf, ""))
}
func protocolStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("protocol status=%d, want=%d (sensitive response omitted)", w.Code, status)
	}
}
func protocolJSON(t *testing.T, w *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
		t.Fatal("invalid protocol JSON")
	}
}
func protocolBody(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func protocolCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("expected one session cookie")
	}
	c := cookies[0]
	if c.Name != browserhttp.CookieName || !c.Secure || !c.HttpOnly || c.Path != "/" || c.Domain != "" || c.SameSite != http.SameSiteLaxMode {
		t.Fatal("unsafe browser cookie")
	}
	return c
}
func (f *protocolFixture) start(t *testing.T) (*http.Cookie, browser.View) {
	t.Helper()
	w := f.browser("GET", "/oauth/authorize?"+f.authorizeQuery().Encode(), "", nil, "")
	protocolStatus(t, w, 303)
	if w.Header().Get("Location") != "/onboarding" {
		t.Fatal("unsafe pending redirect")
	}
	c := protocolCookie(t, w)
	view := f.view(t, c)
	if view.State != "PENDING" || view.UserID != "" || view.ChallengeID != "" || view.TransactionID == "" || view.CSRF == "" || !view.AuthenticationAvailable {
		t.Fatal("invalid pending authority")
	}
	return c, view
}
func (f *protocolFixture) view(t *testing.T, c *http.Cookie) browser.View {
	t.Helper()
	w := f.browser("GET", "/browser/session", "", c, "")
	protocolStatus(t, w, 200)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cacheable session response")
	}
	var view browser.View
	protocolJSON(t, w, &view)
	return view
}
func (f *protocolFixture) login(t *testing.T) protocolLogin {
	t.Helper()
	c, v := f.start(t)
	result := protocolLogin{pending: c, pendingView: v}
	w := f.browser("POST", "/browser/siwe/challenge", protocolBody(t, map[string]string{"address": f.address}), c, v.CSRF)
	protocolStatus(t, w, 200)
	protocolJSON(t, w, &result.challenge)
	if result.challenge.ID == "" || result.challenge.Address != f.address || !strings.Contains(result.challenge.Message, "Chain ID: 5042\n") {
		t.Fatal("invalid issued challenge")
	}
	w = f.browser("POST", "/browser/siwe/verify", protocolBody(t, map[string]string{"challenge_id": result.challenge.ID, "signature": siweSignature(t, result.challenge.Message)}), c, v.CSRF)
	protocolStatus(t, w, 200)
	result.authenticated = protocolCookie(t, w)
	if result.authenticated.Value == c.Value {
		t.Fatal("session not rotated")
	}
	protocolStatus(t, f.browser("GET", "/browser/session", "", c, ""), 401)
	result.view = f.view(t, result.authenticated)
	if result.view.State != "AUTHENTICATED" || result.view.UserID != f.base.scope.ActorID() || result.view.ChallengeID != result.challenge.ID || result.view.TransactionID != v.TransactionID || result.view.CSRF == v.CSRF || result.view.ClientID != f.client.ID || result.view.Scope != oauth.ReadScope || result.view.Resource != oauth.Resource {
		t.Fatal("authentication continuity failed")
	}
	return result
}
func (f *protocolFixture) grant(t *testing.T, l protocolLogin) string {
	t.Helper()
	w := f.browser("POST", "/browser/consent", `{"decision":"grant"}`, l.authenticated, l.view.CSRF)
	protocolStatus(t, w, 200)
	var response struct {
		Redirect string `json:"redirect"`
	}
	protocolJSON(t, w, &response)
	u, err := url.Parse(response.Redirect)
	if err != nil || u.Scheme != "https" || u.Host != "client.example" || u.Path != "/callback" || u.User != nil || u.Fragment != "" || u.Query().Get("iss") != oauth.Issuer || u.Query().Get("state") != "synthetic-client-state" || u.Query().Get("code") == "" {
		t.Fatal("authorization callback binding failed")
	}
	return u.Query().Get("code")
}
func (f *protocolFixture) exchange(code string) url.Values {
	return url.Values{"grant_type": {"authorization_code"}, "client_id": {f.client.ID}, "redirect_uri": {f.client.RedirectURIs[0]}, "resource": {oauth.Resource}, "code": {code}, "code_verifier": {oauthVerifier}}
}
func (f *protocolFixture) redeem(t *testing.T, code string) string {
	t.Helper()
	w := f.browser("POST", "/oauth/token", f.exchange(code).Encode(), nil, "")
	protocolStatus(t, w, 200)
	var token oauth.TokenResponse
	protocolJSON(t, w, &token)
	if token.AccessToken == "" || token.TokenType != "Bearer" || token.Scope != oauth.ReadScope || token.ExpiresIn <= 0 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("invalid token response")
	}
	return token.AccessToken
}
func (f *protocolFixture) mcp(t *testing.T, token, method string, params any) *httptest.ResponseRecorder {
	t.Helper()
	return f.send(f.request("mcp.wizpay.xyz", "POST", "/mcp", protocolBody(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}), nil, "", token))
}
