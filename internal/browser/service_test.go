package browser

import (
	"context"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"sync"
	"testing"
	"time"
)

// Test repository never enters production wiring.
type fixtureRepo struct {
	sync.Mutex
	records     map[string]Session
	transaction oauth.Transaction
	client      oauth.Client
}

func (r *fixtureRepo) CreateBrowserSession(_ context.Context, v Session) error {
	r.Lock()
	defer r.Unlock()
	r.records[v.Digest] = v
	return nil
}
func (r *fixtureRepo) FindBrowserSession(_ context.Context, id string) (Session, error) {
	r.Lock()
	defer r.Unlock()
	v, ok := r.records[id]
	if !ok {
		return v, ErrDenied
	}
	return v, nil
}
func (r *fixtureRepo) FindOAuthTransaction(context.Context, string) (oauth.Transaction, error) {
	return r.transaction, nil
}
func (r *fixtureRepo) FindOAuthClient(context.Context, string) (oauth.Client, error) {
	return r.client, nil
}
func (r *fixtureRepo) RotateBrowserSession(_ context.Context, old, proof string, next Session) error {
	r.Lock()
	defer r.Unlock()
	v, ok := r.records[old]
	if !ok || v.Revoked || v.CSRFHash != proof {
		return ErrDenied
	}
	v.Revoked = true
	r.records[old] = v
	r.records[next.Digest] = next
	return nil
}
func (r *fixtureRepo) RevokeBrowserSession(_ context.Context, id, proof string) error {
	r.Lock()
	defer r.Unlock()
	v := r.records[id]
	if v.CSRFHash != proof || v.Revoked {
		return ErrDenied
	}
	v.Revoked = true
	r.records[id] = v
	return nil
}
func (r *fixtureRepo) DenyBrowserConsent(context.Context, string, string) error { return nil }
func (r *fixtureRepo) CleanupBrowserSessions(context.Context) error             { return nil }

type fixtureFlow struct{}

func (fixtureFlow) Begin(context.Context, oauth.AuthorizationRequest) (oauth.Transaction, error) {
	return oauth.Transaction{}, ErrDenied
}
func (fixtureFlow) CompleteRedirect(context.Context, string, oauth.BrowserAuthorizer) (string, error) {
	panic("pending session reached OAuth completion")
}
func fixture(t *testing.T) (*Service, *fixtureRepo, string) {
	t.Helper()
	now := time.Now()
	r := &fixtureRepo{records: map[string]Session{}, transaction: oauth.Transaction{ID: "transaction", CreatedAt: now, ExpiresAt: now.Add(time.Minute), Request: oauth.AuthorizationRequest{ClientID: "client", RedirectURI: "https://client.example/callback", Scope: oauth.ReadScope, Resource: oauth.Resource}}, client: oauth.Client{ID: "client", Name: "Client", Type: "public", AuthMethod: "none", Status: "ACTIVE", RedirectURIs: []string{"https://client.example/callback"}, Scopes: []string{oauth.ReadScope}, Resources: []string{oauth.Resource}}}
	s, e := NewService(r, r, fixtureFlow{}, nil, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := s.Start(context.Background(), r.transaction)
	if e != nil {
		t.Fatal(e)
	}
	return s, r, raw
}
func TestPendingSessionAndMissingVerifier(t *testing.T) {
	s, _, raw := fixture(t)
	v, e := s.View(context.Background(), raw)
	if e != nil || v.UserID != "" || v.AuthenticationAvailable {
		t.Fatal("pending authority")
	}
	if _, e = s.Authenticate(context.Background(), raw, v.CSRF); e != ErrUnavailable {
		t.Fatal("missing verifier")
	}
	if _, e = s.Grant(context.Background(), raw, v.CSRF); e != ErrUnavailable {
		t.Fatal("pending granted")
	}
}
func TestProofExpiryAndRevocation(t *testing.T) {
	s, r, raw := fixture(t)
	v, _ := s.View(context.Background(), raw)
	other, e := s.Start(context.Background(), r.transaction)
	if e != nil {
		t.Fatal(e)
	}
	for _, proof := range []string{"", "wrong", csrf(other)} {
		if e = s.Logout(context.Background(), raw, proof); e == nil {
			t.Fatal("bad CSRF")
		}
	}
	if e = s.Logout(context.Background(), raw, v.CSRF); e != nil {
		t.Fatal(e)
	}
	if _, e = s.View(context.Background(), raw); e == nil {
		t.Fatal("revoked accepted")
	}
	s.now = func() time.Time { return time.Now().Add(time.Hour) }
	if _, e = s.View(context.Background(), other); e == nil {
		t.Fatal("expired accepted")
	}
}

// Trusted authentication exists only in isolated test code.
type verifierFixture struct {
	principal auth.AuthenticatedPrincipal
	corrupt   bool
}

func (v verifierFixture) VerifyBrowser(_ context.Context, r VerificationRequest) (Verification, error) {
	if v.corrupt {
		r.TransactionID = "different"
	}
	return Verification{Principal: v.principal, SessionReference: r.SessionReference, TransactionID: r.TransactionID, Evidence: "unit-test-only"}, nil
}

type verifiedFlow struct{ repo *fixtureRepo }

func (v verifiedFlow) Begin(context.Context, oauth.AuthorizationRequest) (oauth.Transaction, error) {
	return oauth.Transaction{}, ErrDenied
}
func (v verifiedFlow) CompleteRedirect(ctx context.Context, id string, a oauth.BrowserAuthorizer) (string, error) {
	d, e := a.AuthorizeBrowser(ctx, v.repo.transaction)
	if e != nil {
		return "", e
	}
	if id != v.repo.transaction.ID || d.BrowserSessionReference == "" || len(d.Principal.Permissions()) != 0 {
		return "", ErrDenied
	}
	v.repo.Lock()
	defer v.repo.Unlock()
	for key, s := range v.repo.records {
		if s.Reference == d.BrowserSessionReference {
			if s.Decision != "OPEN" || s.Revoked {
				return "", ErrDenied
			}
			s.Decision = "GRANTED"
			v.repo.records[key] = s
			return "https://client.example/callback?code=test-only", nil
		}
	}
	return "", ErrDenied
}
func TestTrustedFixtureRotationAndConsent(t *testing.T) {
	s, r, raw := fixture(t)
	p, e := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant", ActorID: "user", IdentityProvider: "verified-issuer", ProviderSubject: "subject", ExpiresAt: time.Now().Add(time.Hour), Permissions: []auth.Permission{auth.PermissionDecideApproval}})
	if e != nil {
		t.Fatal(e)
	}
	s.verifier = verifierFixture{principal: p}
	s.oauth = verifiedFlow{r}
	ctx := context.Background()
	view, _ := s.View(ctx, raw)
	next, e := s.Authenticate(ctx, raw, view.CSRF)
	if e != nil || next == raw {
		t.Fatal("rotation", e)
	}
	if _, e = s.View(ctx, raw); e == nil {
		t.Fatal("fixation")
	}
	view, e = s.View(ctx, next)
	if e != nil || view.UserID != "user" {
		t.Fatal("identity", e)
	}
	renewed, e := s.Authenticate(ctx, next, view.CSRF)
	if e != nil {
		t.Fatal("renewal", e)
	}
	view, _ = s.View(ctx, renewed)
	var successes int
	for i := 0; i < 2; i++ {
		if _, e = s.Grant(ctx, renewed, view.CSRF); e == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal("consent replay")
	}
}
func TestVerifierMustBindExactSessionAndTransaction(t *testing.T) {
	s, _, raw := fixture(t)
	p, _ := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant", ActorID: "user", IdentityProvider: "issuer", ProviderSubject: "subject", ExpiresAt: time.Now().Add(time.Hour)})
	s.verifier = verifierFixture{principal: p, corrupt: true}
	view, _ := s.View(context.Background(), raw)
	if _, e := s.Authenticate(context.Background(), raw, view.CSRF); e == nil {
		t.Fatal("unbound proof")
	}
}
