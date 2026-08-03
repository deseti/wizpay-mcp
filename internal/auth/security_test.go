package auth_test

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
)

func principal(t *testing.T, permissions ...auth.Permission) auth.AuthenticatedPrincipal {
	t.Helper()
	value, err := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{
		TenantID: "tenant_1", ActorID: "user_1", IdentityProvider: "issuer.example",
		ProviderSubject: "legacy-unmapped:user_1", ClientID: "client_1", TokenID: "token_1",
		AuthenticatedAt: time.Unix(1_700_000_000, 0), IssuedAt: time.Unix(1_700_000_000, 0),
		ExpiresAt: time.Unix(1_700_000_600, 0), Permissions: permissions,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func identity(t *testing.T, status auth.IdentityStatus) auth.Identity {
	t.Helper()
	value, err := auth.NewIdentity("user_1", "issuer.example", status)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestAuthenticatedPrincipalNormalizesPermissionsAndIsValidated(t *testing.T) {
	value := principal(t, auth.PermissionReadIntent, auth.PermissionCreateIntent, auth.PermissionReadIntent)
	if value.TenantID() != "tenant_1" || value.ActorID() != "user_1" || value.ProviderSubject() != "legacy-unmapped:user_1" {
		t.Fatalf("principal identity = %#v", value)
	}
	got := value.Permissions()
	if len(got) != 2 || got[0] != auth.PermissionCreateIntent || got[1] != auth.PermissionReadIntent {
		t.Fatalf("permissions = %v", got)
	}
	got[0] = auth.PermissionPrepareExecution
	if value.HasPermission(auth.PermissionPrepareExecution) {
		t.Fatal("returned permission slice mutated principal")
	}
}

func TestAuthenticatedPrincipalRejectsMalformedRequiredClaims(t *testing.T) {
	tests := []struct {
		name string
		edit func(*auth.PrincipalParams)
	}{
		{"subject", func(p *auth.PrincipalParams) { p.ProviderSubject = "" }},
		{"tenant", func(p *auth.PrincipalParams) { p.TenantID = "" }},
		{"actor", func(p *auth.PrincipalParams) { p.ActorID = "" }},
		{"provider", func(p *auth.PrincipalParams) { p.IdentityProvider = "bad\nprovider" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := auth.PrincipalParams{TenantID: "tenant_1", ActorID: "user_1", IdentityProvider: "issuer.example", ProviderSubject: "sub_1", ExpiresAt: time.Now().Add(time.Minute)}
			test.edit(&params)
			if _, err := auth.NewAuthenticatedPrincipal(params); err == nil {
				t.Fatal("malformed principal accepted")
			}
		})
	}
}

func TestTrustedRequestContextRoundTripAndScope(t *testing.T) {
	resolved, err := auth.NewTrustedRequest(principal(t, auth.PermissionReadIntent), identity(t, auth.IdentityStatusActive), auth.RequestMetadata{RequestID: "request_1", TraceID: "trace_1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithTrustedRequest(context.Background(), resolved)
	got, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Principal().TenantID() != "tenant_1" || got.Identity().UserID() != "user_1" {
		t.Fatalf("trusted request = %#v", got)
	}
	scope, err := requestauth.StorageScopeFromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if scope.TenantID() != "tenant_1" || scope.ActorID() != "user_1" || scope.RequestID() != "request_1" || scope.TraceID() != "trace_1" {
		t.Fatalf("scope = tenant %q actor %q request %q trace %q", scope.TenantID(), scope.ActorID(), scope.RequestID(), scope.TraceID())
	}
}

func TestMissingTrustedRequestContextIsUnauthenticated(t *testing.T) {
	_, err := auth.TrustedRequestFromContext(context.Background())
	assertCode(t, err, apperrors.CodeAuthenticationRequired)
	_, err = requestauth.StorageScopeFromContext(context.Background())
	assertCode(t, err, apperrors.CodeAuthenticationRequired)
}

func TestTrustedRequestRejectsIdentityMismatchAndInvalidMetadata(t *testing.T) {
	other, _ := auth.NewIdentity("other_user", "issuer.example", auth.IdentityStatusActive)
	if _, err := auth.NewTrustedRequest(principal(t), other, auth.RequestMetadata{RequestID: "request_1"}); err == nil {
		t.Fatal("cross-user identity accepted")
	}
	if _, err := auth.NewTrustedRequest(principal(t), identity(t, auth.IdentityStatusActive), auth.RequestMetadata{RequestID: "caller\nvalue"}); err == nil {
		t.Fatal("invalid request metadata accepted")
	}
}

func TestPermissionAuthorizerFailsClosedAndPreservesWalletOwnership(t *testing.T) {
	authorizer := auth.NewPermissionAuthorizer()
	trusted, err := auth.NewTrustedRequest(principal(t, auth.PermissionReadIntent), identity(t, auth.IdentityStatusActive), auth.RequestMetadata{RequestID: "request_1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := authorizer.Authorize(context.Background(), auth.AuthorizationInput{Request: trusted, Permission: auth.PermissionReadIntent, ResourceOwnerID: "user_1"}); err != nil {
		t.Fatalf("correct permission denied: %v", err)
	}
	assertCode(t, authorizer.Authorize(context.Background(), auth.AuthorizationInput{Request: trusted, Permission: auth.PermissionCreateIntent}), apperrors.CodeAuthorizationRequired)
	assertCode(t, authorizer.Authorize(context.Background(), auth.AuthorizationInput{Request: trusted, Permission: auth.PermissionReadIntent, ResourceOwnerID: "other_user"}), apperrors.CodeAuthorizationRequired)

	wallet := walletContext{owner: "other_user"}
	assertCode(t, authorizer.Authorize(context.Background(), auth.AuthorizationInput{Request: trusted, Permission: auth.PermissionReadIntent, Wallet: wallet}), apperrors.CodeAuthorizationRequired)
}

type walletContext struct{ owner string }

func (w walletContext) BindingID() string   { return "binding_1" }
func (w walletContext) OwnerUserID() string { return w.owner }
func (w walletContext) EnsureAuthorizable(userID string) error {
	if userID != w.owner {
		return apperrors.New(apperrors.CodeWalletMismatch, "Wallet binding does not match the identity.", false, true, true)
	}
	return nil
}

func assertCode(t *testing.T, err error, want apperrors.Code) {
	t.Helper()
	var appErr *apperrors.Error
	if !stderrors.As(err, &appErr) || appErr.Code != want {
		t.Fatalf("error = %v, want code %q", err, want)
	}
}
