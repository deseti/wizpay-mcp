package browser

import (
	"context"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
)

func TestConsentRequiresExactTransactionSnapshot(t *testing.T) {
	s, r, raw := fixture(t)
	p, err := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant", ActorID: "user", IdentityProvider: "verified-issuer", ProviderSubject: "subject", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	s.verifier = verifierFixture{principal: p}
	ctx := context.Background()
	v, err := s.View(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = s.Authenticate(ctx, raw, v.CSRF)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.View(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	adapter := authorizer{s, raw, v.CSRF}
	if _, err = adapter.AuthorizeBrowser(ctx, r.transaction); err != nil {
		t.Fatal("valid snapshot rejected", err)
	}
	for _, field := range []string{"transaction", "client", "redirect", "resource", "scope", "pkce", "state"} {
		t.Run(field, func(t *testing.T) {
			tr := r.transaction
			switch field {
			case "transaction":
				tr.ID = "other"
			case "client":
				tr.Request.ClientID = "other-client"
			case "redirect":
				tr.Request.RedirectURI = "https://other.example/callback"
			case "resource":
				tr.Request.Resource = "https://other.example/mcp"
			case "scope":
				tr.Request.Scope = "approval:decide"
			case "pkce":
				tr.Request.Challenge = "other"
			case "state":
				tr.Request.State = "other"
			}
			if _, err := adapter.AuthorizeBrowser(ctx, tr); err == nil {
				t.Fatal("altered consent authority accepted")
			}
		})
	}
}
