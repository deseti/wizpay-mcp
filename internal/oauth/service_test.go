package oauth

import (
	"context"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"net/url"
	"testing"
	"time"
)

type repositoryStub struct {
	Repository
	client      Client
	transaction Transaction
	token       Token
	begins      int
}

func (r *repositoryStub) FindOAuthClient(context.Context, string) (Client, error) {
	return r.client, nil
}
func (r *repositoryStub) CreateOAuthTransaction(_ context.Context, t Transaction) error {
	r.transaction = t
	r.begins++
	return nil
}
func (r *repositoryStub) FindOAuthTransaction(context.Context, string) (Transaction, error) {
	return r.transaction, nil
}
func (r *repositoryStub) CompleteOAuthAuthorization(context.Context, Transaction, BrowserDecision, Code) error {
	return nil
}
func (r *repositoryStub) ValidateOAuthToken(context.Context, string, time.Time) (Token, error) {
	return r.token, nil
}

type browserFixture struct{ p auth.AuthenticatedPrincipal }

func (b browserFixture) AuthorizeBrowser(_ context.Context, t Transaction) (BrowserDecision, error) {
	return BrowserDecision{Principal: b.p, SessionID: "test-session", SessionExpiresAt: b.p.ExpiresAt(), TransactionID: t.ID, ClientID: t.Request.ClientID, Resource: t.Request.Resource, Scope: t.Request.Scope}, nil
}
func requestFixture() AuthorizationRequest {
	return AuthorizationRequest{ClientID: "client", RedirectURI: "https://client.example/callback", Resource: Resource, Scope: ReadScope, ResponseType: "code", Challenge: Digest("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ"), ChallengeMethod: "S256", State: "client-state"}
}
func clientFixture() Client {
	return Client{ID: "client", Name: "Local fixture", Type: "public", AuthMethod: "none", Status: "ACTIVE", RedirectURIs: []string{"https://client.example/callback"}, Scopes: []string{ReadScope}, Resources: []string{Resource}}
}
func TestAuthorizationValidationAndFailClosedBrowser(t *testing.T) {
	now := time.Now()
	repo := &repositoryStub{client: clientFixture()}
	s, _ := NewService(repo, func() time.Time { return now })
	for _, kind := range []string{"missing-pkce", "plain", "scope", "resource", "redirect", "implicit", "malformed-challenge"} {
		r := requestFixture()
		switch kind {
		case "missing-pkce":
			r.Challenge = ""
		case "plain":
			r.ChallengeMethod = "plain"
		case "scope":
			r.Scope = "approval:decide:human"
		case "resource":
			r.Resource = Issuer
		case "redirect":
			r.RedirectURI = "https://attacker.example"
		case "implicit":
			r.ResponseType = "token"
		case "malformed-challenge":
			r.Challenge = "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
		}
		if _, e := s.Begin(context.Background(), r); e == nil {
			t.Fatalf("accepted %s", kind)
		}
	}
	tr, e := s.Begin(context.Background(), requestFixture())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Complete(context.Background(), tr.ID, nil); e != ErrBrowserUnavailable {
		t.Fatal("browser dependency bypassed")
	}
	p, e := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant", ActorID: "user", IdentityProvider: "issuer", ProviderSubject: "subject", ExpiresAt: now.Add(time.Hour)})
	if e != nil {
		t.Fatal(e)
	}
	raw, e := s.CompleteRedirect(context.Background(), tr.ID, browserFixture{p})
	if e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(raw)
	if e != nil || u.Host != "client.example" || u.Query().Get("iss") != Issuer || u.Query().Get("state") != "client-state" || u.Query().Get("code") == "" {
		t.Fatal("redirect binding missing")
	}
}
func TestOpaqueTokenBoundaryAndPermissionMapping(t *testing.T) {
	now := time.Now()
	base := Token{Issuer: Issuer, Resource: Resource, ClientID: "client", TenantID: "tenant", UserID: "user", IdentityIssuer: "identity-issuer", Subject: "subject", Scope: ReadScope, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute)}
	repo := &repositoryStub{token: base}
	s, _ := NewService(repo, func() time.Time { return now })
	raw, e := random("wmcp_at_")
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.Verify(context.Background(), raw)
	if e != nil {
		t.Fatal(e)
	}
	for _, perm := range []auth.Permission{auth.PermissionCreateIntent, auth.PermissionRequestApproval, auth.PermissionDecideApproval, auth.PermissionConfirmExecution, auth.PermissionPrepareExecution, auth.PermissionAutonomyControl} {
		if p.HasPermission(perm) {
			t.Fatalf("scope granted %s", perm)
		}
	}
	for _, kind := range []string{"issuer", "audience", "expired", "future-issued", "scope", "client"} {
		v := base
		switch kind {
		case "issuer":
			v.Issuer = "other"
		case "audience":
			v.Resource = Issuer
		case "expired":
			v.ExpiresAt = now
		case "future-issued":
			v.IssuedAt = now.Add(time.Second)
		case "scope":
			v.Scope = "approval:decide:human"
		case "client":
			v.ClientID = ""
		}
		repo.token = v
		if _, e = s.Verify(context.Background(), raw); e == nil {
			t.Fatalf("accepted %s", kind)
		}
	}
	repo.token = base
	for _, v := range []string{"browser-session", "eyJ.fake.jwt", "wmcp_ac_abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ"} {
		if _, e = s.Verify(context.Background(), v); e == nil {
			t.Fatal("non-access credential accepted")
		}
	}
}

func TestRFC7636S256Vector(t *testing.T) {
	if Digest("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk") != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatal("S256 RFC7636 vector mismatch")
	}
}
