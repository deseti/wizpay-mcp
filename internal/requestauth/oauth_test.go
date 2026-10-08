package requestauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/oauth"
)

type oauthVerifierStub struct{ p auth.AuthenticatedPrincipal }

func (v oauthVerifierStub) Verify(context.Context, string) (auth.AuthenticatedPrincipal, error) {
	return v.p, nil
}
func TestOAuthChallengesAndScope(t *testing.T) {
	p, e := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant", ActorID: "user", IdentityProvider: "issuer", ProviderSubject: "subject", ExpiresAt: time.Now().Add(time.Hour), Permissions: []auth.Permission{auth.PermissionDecideApproval, auth.PermissionConfirmExecution}})
	if e != nil {
		t.Fatal(e)
	}
	identity, e := auth.NewIdentityWithSubject("user", "issuer", "subject", auth.IdentityStatusActive)
	if e != nil {
		t.Fatal(e)
	}
	m, e := NewOAuthMiddleware(oauthVerifierStub{p}, ResolveIdentityFunc(func(context.Context, auth.AuthenticatedPrincipal) (auth.Identity, error) { return identity, nil }))
	if e != nil {
		t.Fatal(e)
	}
	h := m.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("insufficient scope reached handler") }))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/mcp", nil)
	h.ServeHTTP(w, r)
	if w.Code != 401 || !strings.Contains(w.Header().Get("WWW-Authenticate"), oauth.MetadataURL) || strings.Contains(w.Header().Get("WWW-Authenticate"), "invalid_token") {
		t.Fatal("missing-token challenge")
	}
	r.Header.Set("Authorization", "Bearer fixture")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 || !strings.Contains(w.Header().Get("WWW-Authenticate"), "insufficient_scope") {
		t.Fatal("scope challenge missing")
	}
	r.Header.Add("Authorization", "Bearer another")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("duplicate bearer headers accepted")
	}
}
