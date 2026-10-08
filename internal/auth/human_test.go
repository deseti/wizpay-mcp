package auth

import (
	"context"
	"testing"
	"time"
)

type humanAuthenticatorStub struct{ request TrustedRequest }

func (v humanAuthenticatorStub) AuthenticateHuman(context.Context) (TrustedRequest, error) {
	return v.request, nil
}
func authorityRequest(t *testing.T, tenant, user, client string, permissions ...Permission) TrustedRequest {
	t.Helper()
	p, err := NewAuthenticatedPrincipal(PrincipalParams{TenantID: tenant, ActorID: user, IdentityProvider: "issuer", ProviderSubject: "subject-" + user, ClientID: client, ExpiresAt: time.Now().Add(time.Hour), Permissions: permissions})
	if err != nil {
		t.Fatal(err)
	}
	i, err := NewIdentityWithSubject(user, "issuer", "subject-"+user, IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewTrustedRequest(p, i, RequestMetadata{RequestID: "request", ClientID: "caller-metadata"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestHumanAuthenticationIsSeparateAndScopeBound(t *testing.T) {
	r := authorityRequest(t, "tenant", "user", "client", PermissionRequestApproval, PermissionDecideApproval, PermissionConfirmExecution)
	ctx := WithTrustedRequest(context.Background(), r)
	if RequireHuman(ctx, PermissionDecideApproval) == nil {
		t.Fatal("permission string created human authority")
	}
	if _, err := AuthenticateHumanContext(ctx, nil); err == nil {
		t.Fatal("missing verifier accepted")
	}
	for _, other := range []TrustedRequest{authorityRequest(t, "other-tenant", "user", "client", PermissionDecideApproval), authorityRequest(t, "tenant", "other-user", "client", PermissionDecideApproval), authorityRequest(t, "tenant", "user", "other-client", PermissionDecideApproval)} {
		if _, err := AuthenticateHumanContext(ctx, humanAuthenticatorStub{other}); err == nil {
			t.Fatal("different authority attached")
		}
	}
	human, err := AuthenticateHumanContext(ctx, humanAuthenticatorStub{r})
	if err != nil {
		t.Fatal(err)
	}
	if err = RequireHuman(human, PermissionDecideApproval); err != nil {
		t.Fatal(err)
	}
	if RequireHuman(WithTrustedRequest(human, r), PermissionDecideApproval) == nil {
		t.Fatal("replacement request inherited human authority")
	}
	requestOnly := authorityRequest(t, "tenant", "user", "client", PermissionRequestApproval)
	requestCtx := WithTrustedRequest(context.Background(), requestOnly)
	requestCtx, err = AuthenticateHumanContext(requestCtx, humanAuthenticatorStub{requestOnly})
	if err != nil {
		t.Fatal(err)
	}
	if RequireHuman(requestCtx, PermissionDecideApproval) == nil {
		t.Fatal("request permission allowed decision")
	}
}
func TestVerifiedAgentCannotComeFromRequestMetadata(t *testing.T) {
	r := authorityRequest(t, "tenant", "user", "verified-client", PermissionAutonomyControl)
	id, err := VerifiedAgentID(r)
	if err != nil || id != "verified-client" {
		t.Fatalf("agent=%s err=%v", id, err)
	}
	if _, err = VerifiedAgentID(authorityRequest(t, "tenant", "user", "", PermissionAutonomyControl)); err == nil {
		t.Fatal("metadata established agent")
	}
}
