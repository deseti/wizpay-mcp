package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
	approvalhttp "github.com/deseti/wizpay-mcp/internal/http/approval"
	oauthhttp "github.com/deseti/wizpay-mcp/internal/http/oauth"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
	"github.com/deseti/wizpay-mcp/internal/services"
)

const oauthVerifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

type oauthBrowserFixture struct {
	principal auth.AuthenticatedPrincipal
	sessionID string
}

func (b oauthBrowserFixture) AuthorizeBrowser(_ context.Context, t oauth.Transaction) (oauth.BrowserDecision, error) {
	return oauth.BrowserDecision{Principal: b.principal, SessionID: b.sessionID, SessionExpiresAt: fixtureNow.Add(time.Hour), TransactionID: t.ID, ClientID: t.Request.ClientID, Resource: t.Request.Resource, Scope: t.Request.Scope}, nil
}

type oauthFixture struct {
	f           fixture
	service     *oauth.Service
	browser     oauthBrowserFixture
	request     oauth.AuthorizationRequest
	now         *time.Time
	transaction oauth.Transaction
	code        string
}

func newOAuthFixture(t *testing.T) oauthFixture {
	t.Helper()
	ctx := context.Background()
	f := createBaseFixture(t, false)
	now := fixtureNow.Add(time.Minute)
	c := oauth.Client{ID: unique("oauth-client"), Name: "Isolated test client", Type: "public", AuthMethod: "none", Status: "ACTIVE", RedirectURIs: []string{"https://client.example/callback"}, Scopes: []string{oauth.ReadScope}, Resources: []string{oauth.Resource}}
	if e := integrationStore.RegisterOAuthClient(ctx, c); e != nil {
		t.Fatal(e)
	}
	store := *integrationStore
	store.now = func() time.Time { return now }
	service, e := oauth.NewService(&store, store.now)
	if e != nil {
		t.Fatal(e)
	}
	p, e := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: f.scope.TenantID(), ActorID: f.scope.ActorID(), IdentityProvider: f.identity.Provider(), ProviderSubject: f.identity.ProviderSubject(), ClientID: "trusted-browser-fixture", ExpiresAt: fixtureNow.Add(time.Hour)})
	if e != nil {
		t.Fatal(e)
	}
	request := oauth.AuthorizationRequest{ClientID: c.ID, RedirectURI: c.RedirectURIs[0], Resource: oauth.Resource, Scope: oauth.ReadScope, ResponseType: "code", Challenge: oauth.Digest(oauthVerifier), ChallengeMethod: "S256", State: "fixture-state"}
	tr, e := service.Begin(ctx, request)
	if e != nil {
		t.Fatal(e)
	}
	browser := oauthBrowserFixture{p, unique("browser-session-reference")}
	code, e := service.Complete(ctx, tr.ID, browser)
	if e != nil {
		t.Fatal(e)
	}
	return oauthFixture{f: f, service: service, browser: browser, request: request, now: &now, transaction: tr, code: code}
}
func (f oauthFixture) exchange() oauth.Exchange {
	return oauth.Exchange{ClientID: f.request.ClientID, RedirectURI: f.request.RedirectURI, Resource: oauth.Resource, Code: f.code, Verifier: oauthVerifier, GrantType: "authorization_code"}
}
func TestOAuthCodeBindingsAndReplay(t *testing.T) {
	for _, kind := range []string{"valid", "verifier", "missing-verifier", "client", "redirect", "resource", "expired"} {
		t.Run(kind, func(t *testing.T) {
			f := newOAuthFixture(t)
			x := f.exchange()
			switch kind {
			case "verifier":
				x.Verifier = strings.Repeat("a", 43)
			case "missing-verifier":
				x.Verifier = ""
			case "client":
				x.ClientID = unique("other-client")
				other := oauth.Client{ID: x.ClientID, Name: "Other local client", Type: "public", AuthMethod: "none", Status: "ACTIVE", RedirectURIs: []string{x.RedirectURI}, Scopes: []string{oauth.ReadScope}, Resources: []string{oauth.Resource}}
				if e := integrationStore.RegisterOAuthClient(context.Background(), other); e != nil {
					t.Fatal(e)
				}
			case "redirect":
				x.RedirectURI = "https://other.example/callback"
			case "resource":
				x.Resource = oauth.Issuer
			case "expired":
				*f.now = f.now.Add(oauth.CodeLifetime + time.Second)
			}
			token, e := f.service.Exchange(context.Background(), x)
			if kind != "valid" {
				if e == nil {
					t.Fatalf("accepted %s", kind)
				}
				if kind != "expired" {
					if _, e = f.service.Exchange(context.Background(), f.exchange()); e != nil {
						t.Fatal("invalid attempt consumed valid code")
					}
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.service.Exchange(context.Background(), x); e == nil {
				t.Fatal("code replay accepted")
			}
			if _, e = f.service.Verify(context.Background(), token.AccessToken); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestOAuthConcurrentCodeRedemption(t *testing.T) {
	f := newOAuthFixture(t)
	var success atomic.Int32
	var wg sync.WaitGroup
	for n := 0; n < 12; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := f.service.Exchange(context.Background(), f.exchange()); e == nil {
				success.Add(1)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 {
		t.Fatalf("redemption successes=%d", success.Load())
	}
}
func TestOAuthAuthorityRevocationAndIdentity(t *testing.T) {
	for _, kind := range []string{"consent", "session", "token", "client", "identity", "expired", "cross-user", "cross-tenant"} {
		t.Run(kind, func(t *testing.T) {
			f := newOAuthFixture(t)
			ctx := context.Background()
			token, e := f.service.Exchange(ctx, f.exchange())
			if e != nil {
				t.Fatal(e)
			}
			var consent string
			if e = integrationPool.QueryRow(ctx, `SELECT consent_id FROM oauth_access_tokens WHERE token_digest=$1`, oauth.Digest(token.AccessToken)).Scan(&consent); e != nil {
				t.Fatal(e)
			}
			r := oauth.Revocation{TenantID: f.f.scope.TenantID(), UserID: f.f.scope.ActorID(), ID: consent, Kind: "consent"}
			switch kind {
			case "session":
				r.Kind = "session"
				r.ID = f.browser.sessionID
			case "token":
				r.Kind = "token"
				r.ID = oauth.Digest(token.AccessToken)
			case "cross-user":
				r.UserID = "other"
			case "cross-tenant":
				r.TenantID = "other"
			}
			switch kind {
			case "client":
				_, e = integrationPool.Exec(ctx, `UPDATE oauth_clients SET status='REVOKED' WHERE client_id=$1`, f.request.ClientID)
			case "identity":
				_, e = integrationPool.Exec(ctx, `UPDATE identities SET status='REVOKED',lifecycle_version=lifecycle_version+1 WHERE tenant_id=$1 AND user_id=$2`, r.TenantID, r.UserID)
			case "expired":
				*f.now = f.now.Add(oauth.TokenLifetime + time.Second)
			default:
				e = integrationStore.RevokeOAuthAuthority(ctx, r)
			}
			if kind == "cross-user" || kind == "cross-tenant" {
				if e == nil {
					t.Fatal("cross-identity revocation accepted")
				}
				if _, e = f.service.Verify(ctx, token.AccessToken); e != nil {
					t.Fatal("cross-identity request revoked victim")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.service.Verify(ctx, token.AccessToken); e == nil {
				t.Fatalf("%s authority accepted", kind)
			}
		})
	}
}
func TestOAuthConsentCannotBeRebound(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	// Completed state is one-shot; a trusted fixture cannot issue a second code.
	if _, e := f.service.Complete(ctx, f.transaction.ID, f.browser); e == nil {
		t.Fatal("consent completion replay")
	}
	if _, e := integrationPool.Exec(ctx, `UPDATE oauth_codes SET client_id=$2 WHERE transaction_id=$1`, f.transaction.ID, unique("other-client")); e == nil {
		t.Fatal("code client substitution")
	}
	token, e := f.service.Exchange(ctx, f.exchange())
	if e != nil {
		t.Fatal(e)
	}
	if _, e := integrationPool.Exec(ctx, `UPDATE oauth_access_tokens SET user_id='other' WHERE token_digest=$1`, oauth.Digest(token.AccessToken)); e == nil {
		t.Fatal("token owner substitution")
	}
	if _, e := integrationPool.Exec(ctx, `UPDATE oauth_consents SET scope='approval:decide:human' WHERE tenant_id=$1 AND user_id=$2`, f.f.scope.TenantID(), f.f.scope.ActorID()); e == nil {
		t.Fatal("consent scope escalation")
	}
}
func TestOAuthHTTPAndMCPHumanIsolation(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	token, e := f.service.Exchange(ctx, f.exchange())
	if e != nil {
		t.Fatal(e)
	}
	m, e := requestauth.NewOAuthMiddleware(f.service, requestauth.RepositoryResolver{Repository: integrationStore})
	if e != nil {
		t.Fatal(e)
	}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Header.Get("Authorization") != "" {
			t.Fatal("bearer forwarded")
		}
		if auth.RequireHuman(r.Context(), auth.PermissionDecideApproval) == nil || auth.RequireHuman(r.Context(), auth.PermissionConfirmExecution) == nil {
			t.Fatal("OAuth became human authority")
		}
		p, _ := auth.TrustedRequestFromContext(r.Context())
		if p.Principal().ClientID() != f.request.ClientID || p.Principal().ActorID() != f.f.scope.ActorID() {
			t.Fatal("identity substitution")
		}
		w.WriteHeader(204)
	})
	handler := oauthhttp.Routes(oauthhttp.NewHandler(f.service), m.Wrap(next))
	for _, raw := range []string{token.AccessToken, "browser-session", "eyJ.fake.jwt", ""} {
		w := httptest.NewRecorder()
		r := oauthTestRequest("POST", oauth.Resource, nil)
		if raw != "" {
			r.Header.Set("Authorization", "Bearer "+raw)
			r.Header.Set("X-User-ID", "other-user")
			r.Header.Set("X-Tenant-ID", "other-tenant")
			r.Header.Set("X-Client-ID", "other-client")
		}
		handler.ServeHTTP(w, r)
		if raw == token.AccessToken {
			if w.Code != 204 || !called {
				t.Fatal("valid MCP access denied")
			}
		} else if w.Code != 401 || !strings.Contains(w.Header().Get("WWW-Authenticate"), oauth.MetadataURL) {
			t.Fatal("invalid bearer challenge")
		}
	}
	r := oauthTestRequest("POST", oauth.Resource, nil)
	r.Header.Set("Authorization", "Bearer "+token.AccessToken)
	r.Header.Set("Origin", "https://attacker.example")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin MCP request")
	}
	// Registered, valid authorize request still stops before browser authentication.
	q := url.Values{"client_id": {f.request.ClientID}, "redirect_uri": {f.request.RedirectURI}, "resource": {oauth.Resource}, "scope": {oauth.ReadScope}, "response_type": {"code"}, "code_challenge": {f.request.Challenge}, "code_challenge_method": {"S256"}}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, oauthTestRequest("GET", oauth.Issuer+"/oauth/authorize?"+q.Encode(), nil))
	if w.Code != 503 || w.Header().Get("Location") != "" {
		t.Fatal("untrusted browser completed OAuth")
	}
}

func oauthTestRequest(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.URL.Scheme = ""
	r.URL.Host = ""
	r.RequestURI = r.URL.RequestURI()
	return r
}

func TestOAuthHTTPTokenExchangeAndHumanEndpoints(t *testing.T) {
	f := newOAuthFixture(t)
	values := url.Values{"grant_type": {"authorization_code"}, "client_id": {f.request.ClientID}, "redirect_uri": {f.request.RedirectURI}, "resource": {oauth.Resource}, "code": {f.code}, "code_verifier": {oauthVerifier}}
	h := oauthhttp.NewHandler(f.service)
	server := httptest.NewServer(h)
	defer server.Close()
	r, err := http.NewRequest("POST", server.URL+"/oauth/token", strings.NewReader(values.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	r.Host = "connect.wizpay.xyz"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("token response failed")
	}
	var token oauth.TokenResponse
	if e := json.NewDecoder(response.Body).Decode(&token); e != nil {
		t.Fatal(e)
	}
	if token.TokenType != "Bearer" || token.Scope != oauth.ReadScope || token.ExpiresIn <= 0 {
		t.Fatal("invalid token profile")
	}
	m, e := requestauth.NewOAuthMiddleware(f.service, requestauth.RepositoryResolver{Repository: integrationStore})
	if e != nil {
		t.Fatal(e)
	}
	approvalService := &services.PersistedApprovalService{Authorizer: auth.NewPermissionAuthorizer(), Approvals: integrationStore, Intents: integrationStore, Wallets: integrationStore, Now: func() time.Time { return *f.now }}
	approvals, e := approvalhttp.NewHandler(approvalService)
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"decision", "authorize-execution"} {
		r := oauthTestRequest("POST", "https://mcp.wizpay.xyz/approval/"+f.f.approval.ApprovalID()+"/"+path, strings.NewReader(`{"is_human":true}`))
		r.Header.Set("Authorization", "Bearer "+token.AccessToken)
		w := httptest.NewRecorder()
		m.Wrap(approvals).ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("OAuth exercised human-only endpoint")
		}
	}
	// A token scope never confers wallet or execution permissions.
	p, e := f.service.Verify(context.Background(), token.AccessToken)
	if e != nil {
		t.Fatal(e)
	}
	for _, perm := range []auth.Permission{auth.PermissionCreateIntent, auth.PermissionRequestApproval, auth.PermissionPrepareExecution, auth.PermissionAutonomyControl, auth.PermissionDecideApproval, auth.PermissionConfirmExecution} {
		if p.HasPermission(perm) {
			t.Fatal("OAuth financial scope escalation")
		}
	}
}

func TestOAuthRevokedConsentBlocksUnredeemedCode(t *testing.T) {
	f := newOAuthFixture(t)
	var consent string
	if e := integrationPool.QueryRow(context.Background(), `SELECT consent_id FROM oauth_codes WHERE code_digest=$1`, oauth.Digest(f.code)).Scan(&consent); e != nil {
		t.Fatal(e)
	}
	if e := integrationStore.RevokeOAuthAuthority(context.Background(), oauth.Revocation{TenantID: f.f.scope.TenantID(), UserID: f.f.scope.ActorID(), ID: consent, Kind: "consent"}); e != nil {
		t.Fatal(e)
	}
	if _, e := f.service.Exchange(context.Background(), f.exchange()); e == nil {
		t.Fatal("revoked consent minted token")
	}
}

func TestOAuthWalletRestrictionFailsClosed(t *testing.T) {
	for _, issued := range []bool{false, true} {
		f := newOAuthFixture(t)
		ctx := context.Background()
		var token oauth.TokenResponse
		var e error
		if issued {
			token, e = f.service.Exchange(ctx, f.exchange())
			if e != nil {
				t.Fatal(e)
			}
		}
		var consent string
		if e = integrationPool.QueryRow(ctx, `SELECT consent_id FROM oauth_codes WHERE transaction_id=$1`, f.transaction.ID).Scan(&consent); e != nil {
			t.Fatal(e)
		}
		b := f.f.binding
		_, e = integrationPool.Exec(ctx, `INSERT INTO oauth_consent_wallets(tenant_id,user_id,consent_id,wallet_binding_id,wallet_binding_version,wallet_id,wallet_address,chain_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, f.f.scope.TenantID(), f.f.scope.ActorID(), consent, b.BindingID(), b.Version(), b.WalletID(), b.Address(), b.ChainID())
		if e != nil {
			t.Fatal(e)
		}
		if issued {
			if _, e = f.service.Verify(ctx, token.AccessToken); e == nil {
				t.Fatal("wallet restriction ignored on access")
			}
		} else {
			if _, e = f.service.Exchange(ctx, f.exchange()); e == nil {
				t.Fatal("wallet restriction ignored on issuance")
			}
		}
	}
}

func TestOAuthLockedAuthorityRevalidation(t *testing.T) {
	for _, kind := range []string{"code-expiry", "consent-expiry", "session-expiry", "consent-revocation", "session-revocation", "client-revocation"} {
		t.Run(kind, func(t *testing.T) {
			f := newOAuthFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			store := *integrationStore
			var clock atomic.Int64
			clock.Store(f.now.UnixNano())
			store.now = func() time.Time { return time.Unix(0, clock.Load()).UTC() }
			service, err := oauth.NewService(&store, store.now)
			if err != nil {
				t.Fatal(err)
			}
			var consent, session string
			var expiry time.Time
			if err := integrationPool.QueryRow(ctx, `SELECT consent_id,session_id,expires_at FROM oauth_codes WHERE code_digest=$1`, oauth.Digest(f.code)).Scan(&consent, &session, &expiry); err != nil {
				t.Fatal(err)
			}
			blocker, err := integrationPool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback(context.Background())
			if _, err = blocker.Exec(ctx, `SELECT code_digest FROM oauth_codes WHERE code_digest=$1 FOR UPDATE`, oauth.Digest(f.code)); err != nil {
				t.Fatal(err)
			}
			// Pending revocation is deliberately invisible to the redemption's initial
			// snapshot. Its locked row must be re-evaluated after the blocker commits.
			switch kind {
			case "consent-revocation":
				_, err = blocker.Exec(ctx, `UPDATE oauth_consents SET revoked_at=$2 WHERE consent_id=$1`, consent, *f.now)
			case "session-revocation":
				_, err = blocker.Exec(ctx, `UPDATE oauth_sessions SET revoked_at=$2 WHERE session_id=$1`, session, *f.now)
			case "client-revocation":
				_, err = blocker.Exec(ctx, `UPDATE oauth_clients SET status='REVOKED' WHERE client_id=$1`, f.request.ClientID)
			case "consent-expiry":
				err = blocker.QueryRow(ctx, `SELECT expires_at FROM oauth_consents WHERE consent_id=$1`, consent).Scan(&expiry)
			case "session-expiry":
				err = blocker.QueryRow(ctx, `SELECT expires_at FROM oauth_sessions WHERE session_id=$1`, session).Scan(&expiry)
			}
			if err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				token, e := service.Exchange(ctx, f.exchange())
				if token.AccessToken != "" {
					result <- fmt.Errorf("issued token after invalid authority")
					return
				}
				result <- e
			}()
			// Observe a real PostgreSQL lock wait before advancing the trusted test clock.
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			for {
				var waiting bool
				if err = integrationPool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::int=ANY(pg_blocking_pids(pid)))`, int32(blocker.Conn().PgConn().PID())).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("redemption did not reach lock wait")
				case <-ticker.C:
				}
			}
			if strings.HasSuffix(kind, "expiry") {
				clock.Store(expiry.Add(time.Nanosecond).UnixNano())
			}
			if err = blocker.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-result; err == nil {
				t.Fatal("invalid locked authority accepted")
			}
			var consumed bool
			var tokens, audits int
			if err = integrationPool.QueryRow(ctx, `SELECT consumed_at IS NOT NULL FROM oauth_codes WHERE code_digest=$1`, oauth.Digest(f.code)).Scan(&consumed); err != nil {
				t.Fatal(err)
			}
			if err = integrationPool.QueryRow(ctx, `SELECT count(*) FROM oauth_access_tokens WHERE consent_id=$1`, consent).Scan(&tokens); err != nil {
				t.Fatal(err)
			}
			if err = integrationPool.QueryRow(ctx, `SELECT count(*) FROM oauth_audit WHERE consent_id=$1 AND event_type='TOKEN_ISSUED'`, consent).Scan(&audits); err != nil {
				t.Fatal(err)
			}
			if consumed || tokens != 0 || audits != 0 {
				t.Fatalf("partial issuance: consumed=%v tokens=%d audits=%d", consumed, tokens, audits)
			}
		})
	}
}
