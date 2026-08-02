package wallet

import (
	stderrors "errors"
	"testing"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

var bindingCreatedAt = time.Date(2026, time.August, 2, 10, 0, 0, 0, time.UTC)

func pendingBinding(t *testing.T) Binding {
	t.Helper()
	binding, err := NewBinding(BindingParams{
		BindingID:             "binding_123",
		Version:               1,
		UserID:                "user_123",
		Provider:              "example-provider",
		ProviderUserReference: "provider-user-123",
		WalletID:              "wallet_123",
		Address:               "canonical-wallet-address",
		ChainID:               "5042002",
		Network:               "example-testnet",
		Status:                BindingStatusPending,
		CreatedAt:             bindingCreatedAt,
	})
	if err != nil {
		t.Fatalf("NewBinding() error = %v", err)
	}
	return binding
}

func activeBinding(t *testing.T) Binding {
	t.Helper()
	binding, err := pendingBinding(t).Transition(BindingStatusActive, bindingCreatedAt.Add(time.Minute), "verification_123")
	if err != nil {
		t.Fatalf("Transition(ACTIVE) error = %v", err)
	}
	return binding
}

func TestBindingValidTransition(t *testing.T) {
	binding := activeBinding(t)
	if binding.Status() != BindingStatusActive || binding.Version() != 2 {
		t.Fatalf("active binding status/version = %s/%d", binding.Status(), binding.Version())
	}
	if binding.VerificationReference() != "verification_123" || binding.VerifiedAt().IsZero() {
		t.Fatalf("active binding verification metadata = %q %s", binding.VerificationReference(), binding.VerifiedAt())
	}
	if err := binding.EnsureAuthorizable("user_123"); err != nil {
		t.Fatalf("EnsureAuthorizable() error = %v", err)
	}
}

func TestBindingRejectsInvalidTransition(t *testing.T) {
	binding := activeBinding(t)
	if _, err := binding.Transition(BindingStatusPending, bindingCreatedAt.Add(2*time.Minute), ""); err == nil {
		t.Fatal("active binding transitioned back to pending")
	}
}

func TestBindingRejectsWalletMismatch(t *testing.T) {
	binding := activeBinding(t)
	err := binding.EnsureMatches(Reference{
		UserID:   binding.OwnerUserID(),
		WalletID: binding.WalletID(),
		Address:  "different-address",
		ChainID:  binding.ChainID(),
	})
	assertWalletCode(t, err, apperrors.CodeWalletMismatch)
}

func TestRevokedBindingCannotAuthorizeOrTransition(t *testing.T) {
	binding := activeBinding(t)
	revoked, err := binding.Transition(BindingStatusRevoked, bindingCreatedAt.Add(2*time.Minute), "")
	if err != nil {
		t.Fatalf("Transition(REVOKED) error = %v", err)
	}
	assertWalletCode(t, revoked.EnsureAuthorizable("user_123"), apperrors.CodeWalletRevoked)
	if _, err := revoked.Transition(BindingStatusActive, bindingCreatedAt.Add(3*time.Minute), "verification_456"); err == nil {
		t.Fatal("revoked binding transitioned back to active")
	}
}

func TestPendingBindingCannotAuthorize(t *testing.T) {
	assertWalletCode(t, pendingBinding(t).EnsureAuthorizable("user_123"), apperrors.CodeWalletNotBound)
}

func TestBindingDuplicateKeyIsDeterministic(t *testing.T) {
	first := pendingBinding(t)
	second := pendingBinding(t)
	firstKey, err := first.DuplicateKey()
	if err != nil {
		t.Fatalf("first DuplicateKey() error = %v", err)
	}
	secondKey, err := second.DuplicateKey()
	if err != nil {
		t.Fatalf("second DuplicateKey() error = %v", err)
	}
	if firstKey != secondKey {
		t.Fatalf("duplicate keys differ: %q != %q", firstKey, secondKey)
	}
}

func assertWalletCode(t *testing.T, err error, want apperrors.Code) {
	t.Helper()
	var appError *apperrors.Error
	if !stderrors.As(err, &appError) {
		t.Fatalf("error = %v, want application error", err)
	}
	if appError.Code != want {
		t.Fatalf("error code = %q, want %q", appError.Code, want)
	}
}
