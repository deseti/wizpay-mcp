package auth

import (
	stderrors "errors"
	"testing"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func TestNewIdentity(t *testing.T) {
	identity, err := NewIdentity("user_123", "example-provider", IdentityStatusActive)
	if err != nil {
		t.Fatalf("NewIdentity() error = %v", err)
	}
	if identity.UserID() != "user_123" || identity.Provider() != "example-provider" || identity.Status() != IdentityStatusActive {
		t.Fatalf("NewIdentity() = user %q provider %q status %q", identity.UserID(), identity.Provider(), identity.Status())
	}
	if err := identity.EnsureAuthorizable(); err != nil {
		t.Fatalf("EnsureAuthorizable() error = %v", err)
	}
}

func TestNewIdentityRejectsInvalidStatus(t *testing.T) {
	if _, err := NewIdentity("user_123", "example-provider", IdentityStatus("INVALID")); err == nil {
		t.Fatal("NewIdentity() accepted invalid status")
	}
}

func TestRevokedIdentityCannotAuthorizeOrTransition(t *testing.T) {
	identity, err := NewIdentity("user_123", "example-provider", IdentityStatusActive)
	if err != nil {
		t.Fatalf("NewIdentity() error = %v", err)
	}
	revoked, err := identity.Transition(IdentityStatusRevoked)
	if err != nil {
		t.Fatalf("Transition(REVOKED) error = %v", err)
	}

	assertApplicationCode(t, revoked.EnsureAuthorizable(), apperrors.CodeIdentityRevoked)
	if _, err := revoked.Transition(IdentityStatusActive); err == nil {
		t.Fatal("revoked identity transitioned back to active")
	}
}

func assertApplicationCode(t *testing.T, err error, want apperrors.Code) {
	t.Helper()
	var appError *apperrors.Error
	if !stderrors.As(err, &appError) {
		t.Fatalf("error = %v, want application error", err)
	}
	if appError.Code != want {
		t.Fatalf("error code = %q, want %q", appError.Code, want)
	}
}
