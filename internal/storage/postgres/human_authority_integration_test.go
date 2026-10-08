package postgres

import (
	"context"
	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/services"
	"testing"
	"time"
)

type localHumanVerifier struct{ request auth.TrustedRequest }

func (v localHumanVerifier) AuthenticateHuman(context.Context) (auth.TrustedRequest, error) {
	return v.request, nil
}
func TestWP1HumanApprovalIsUserAndTenantScoped(t *testing.T) {
	for _, kind := range []string{"mcp", "other-user", "other-tenant", "valid", "revoked", "expired"} {
		t.Run(kind, func(t *testing.T) {
			f := createBaseFixture(t, false)
			user, tenant := f.scope.ActorID(), f.scope.TenantID()
			if kind == "other-user" {
				user = "other-user"
			}
			if kind == "other-tenant" {
				tenant = "other-tenant"
			}
			i, err := auth.NewIdentityWithSubject(user, f.identity.Provider(), f.identity.ProviderSubject(), auth.IdentityStatusActive)
			if err != nil {
				t.Fatal(err)
			}
			p, err := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: tenant, ActorID: user, IdentityProvider: i.Provider(), ProviderSubject: i.ProviderSubject(), ClientID: "local-client", ExpiresAt: fixtureNow.Add(time.Hour), Permissions: []auth.Permission{auth.PermissionRequestApproval, auth.PermissionDecideApproval, auth.PermissionConfirmExecution}})
			if err != nil {
				t.Fatal(err)
			}
			r, err := auth.NewTrustedRequest(p, i, auth.RequestMetadata{RequestID: unique("request")})
			if err != nil {
				t.Fatal(err)
			}
			ctx := auth.WithTrustedRequest(context.Background(), r)
			if kind != "mcp" {
				ctx, err = auth.AuthenticateHumanContext(ctx, localHumanVerifier{r})
				if err != nil {
					t.Fatal(err)
				}
			}
			s := services.PersistedApprovalService{Intents: integrationStore, Approvals: integrationStore, Wallets: integrationStore, Audit: integrationStore, Authorizer: auth.NewPermissionAuthorizer(), Now: func() time.Time { return fixtureNow.Add(time.Minute) }}
			if kind == "revoked" {
				b, err := f.binding.Transition("REVOKED", fixtureNow.Add(time.Second), "")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = integrationStore.UpdateBinding(ctx, f.scope, b, f.binding.Version()); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "expired" {
				s.Now = func() time.Time { return f.approval.ExpiresAt().Add(time.Second) }
			}
			_, err = s.DecideApproval(ctx, f.approval.ApprovalID(), approvals.DecisionApproved)
			if kind == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid decision accepted")
			}
			_, err = s.AuthorizeExecution(ctx, f.approval.ApprovalID(), f.intent.IntentID(), f.binding.BindingID(), f.binding.Version())
			if kind == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid signing handoff accepted")
			}
		})
	}
}
