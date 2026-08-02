// Package wallet defines provider-neutral wallet ownership metadata and
// lifecycle rules. It contains no key, signing, or transaction capability.
package wallet

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

const (
	maxBindingFieldLength = 256
	maxChainIDLength      = 20
)

// BindingParams contains metadata required to construct or restore a binding.
// ProviderUserReference is an opaque stable identifier, never a credential.
type BindingParams struct {
	BindingID             string
	Version               uint64
	UserID                string
	Provider              string
	ProviderUserReference string
	WalletID              string
	Address               string
	ChainID               string
	Network               string
	Status                BindingStatus
	VerificationReference string
	CreatedAt             time.Time
	VerifiedAt            time.Time
	RevokedAt             time.Time
}

// Binding represents the ownership relationship between an application user
// and one provider-managed wallet on one explicit network.
type Binding struct {
	bindingID             string
	version               uint64
	userID                string
	provider              string
	providerUserReference string
	walletID              string
	address               string
	chainID               string
	network               string
	status                BindingStatus
	verificationReference string
	createdAt             time.Time
	verifiedAt            time.Time
	revokedAt             time.Time
}

// NewBinding validates metadata and creates a binding value without contacting
// a provider. Network-specific address canonicalization must happen before this
// constructor is called by a future provider adapter.
func NewBinding(params BindingParams) (Binding, error) {
	binding := Binding{
		bindingID:             strings.TrimSpace(params.BindingID),
		version:               params.Version,
		userID:                strings.TrimSpace(params.UserID),
		provider:              strings.TrimSpace(params.Provider),
		providerUserReference: strings.TrimSpace(params.ProviderUserReference),
		walletID:              strings.TrimSpace(params.WalletID),
		address:               strings.TrimSpace(params.Address),
		chainID:               strings.TrimSpace(params.ChainID),
		network:               strings.TrimSpace(params.Network),
		status:                params.Status,
		verificationReference: strings.TrimSpace(params.VerificationReference),
		createdAt:             params.CreatedAt.UTC(),
		verifiedAt:            params.VerifiedAt.UTC(),
		revokedAt:             params.RevokedAt.UTC(),
	}
	if err := binding.Validate(); err != nil {
		return Binding{}, err
	}
	return binding, nil
}

// Validate checks binding metadata and lifecycle timestamp invariants.
func (b Binding) Validate() error {
	fields := []struct {
		name  string
		value string
	}{
		{name: "binding ID", value: b.bindingID},
		{name: "user ID", value: b.userID},
		{name: "wallet provider", value: b.provider},
		{name: "provider user reference", value: b.providerUserReference},
		{name: "wallet ID", value: b.walletID},
		{name: "wallet address", value: b.address},
		{name: "network", value: b.network},
	}
	for _, field := range fields {
		if field.value == "" {
			return fmt.Errorf("%s is required", field.name)
		}
		if len(field.value) > maxBindingFieldLength {
			return fmt.Errorf("%s exceeds %d characters", field.name, maxBindingFieldLength)
		}
		if strings.IndexFunc(field.value, unicode.IsControl) >= 0 {
			return fmt.Errorf("%s contains control characters", field.name)
		}
	}
	if b.version == 0 {
		return fmt.Errorf("binding version must be at least 1")
	}
	if err := validateChainID(b.chainID); err != nil {
		return err
	}
	if !b.status.Valid() {
		return fmt.Errorf("invalid wallet binding status %q", b.status)
	}
	if b.createdAt.IsZero() {
		return fmt.Errorf("binding creation time is required")
	}

	switch b.status {
	case BindingStatusPending:
		if !b.verifiedAt.IsZero() || !b.revokedAt.IsZero() || b.verificationReference != "" {
			return fmt.Errorf("pending binding cannot contain verification or revocation metadata")
		}
	case BindingStatusActive:
		if b.verifiedAt.IsZero() || b.verificationReference == "" {
			return fmt.Errorf("active binding requires verification time and reference")
		}
		if !b.revokedAt.IsZero() {
			return fmt.Errorf("active binding cannot contain revocation time")
		}
	case BindingStatusRevoked:
		if b.revokedAt.IsZero() {
			return fmt.Errorf("revoked binding requires revocation time")
		}
	}

	if !b.verifiedAt.IsZero() && b.verifiedAt.Before(b.createdAt) {
		return fmt.Errorf("verification time cannot precede creation time")
	}
	if !b.revokedAt.IsZero() && b.revokedAt.Before(b.createdAt) {
		return fmt.Errorf("revocation time cannot precede creation time")
	}
	if !b.revokedAt.IsZero() && !b.verifiedAt.IsZero() && b.revokedAt.Before(b.verifiedAt) {
		return fmt.Errorf("revocation time cannot precede verification time")
	}
	if len(b.verificationReference) > maxBindingFieldLength {
		return fmt.Errorf("verification reference exceeds %d characters", maxBindingFieldLength)
	}
	return nil
}

// Transition returns a new binding value. Repeated delivery of the current
// state is idempotent. Activating requires an opaque verification reference;
// revocation is terminal.
func (b Binding) Transition(next BindingStatus, at time.Time, verificationReference string) (Binding, error) {
	if err := b.Validate(); err != nil {
		return Binding{}, err
	}
	if err := validateTransition(b.status, next); err != nil {
		return Binding{}, err
	}
	if next == b.status {
		return b, nil
	}
	if at.IsZero() || at.Before(b.createdAt) {
		return Binding{}, fmt.Errorf("transition time must not precede binding creation")
	}
	if b.version == ^uint64(0) {
		return Binding{}, fmt.Errorf("binding version cannot advance")
	}

	nextBinding := b
	nextBinding.version++
	nextBinding.status = next
	switch next {
	case BindingStatusActive:
		nextBinding.verificationReference = strings.TrimSpace(verificationReference)
		nextBinding.verifiedAt = at.UTC()
	case BindingStatusRevoked:
		nextBinding.revokedAt = at.UTC()
	}
	if err := nextBinding.Validate(); err != nil {
		return Binding{}, err
	}
	return nextBinding, nil
}

// Reference contains the exact wallet metadata expected by a future request or
// authorization record.
type Reference struct {
	UserID   string
	WalletID string
	Address  string
	ChainID  string
}

// EnsureMatches fails closed if any ownership-critical wallet field differs.
func (b Binding) EnsureMatches(reference Reference) error {
	if err := b.Validate(); err != nil {
		return apperrors.Wrap(apperrors.CodeValidationError, "Wallet binding is invalid.", false, true, true, err)
	}
	if b.userID != strings.TrimSpace(reference.UserID) ||
		b.walletID != strings.TrimSpace(reference.WalletID) ||
		b.address != strings.TrimSpace(reference.Address) ||
		b.chainID != strings.TrimSpace(reference.ChainID) {
		return apperrors.New(apperrors.CodeWalletMismatch, "Wallet binding does not match the request.", false, true, true)
	}
	return nil
}

// EnsureAuthorizable checks ownership and lifecycle state only. It does not
// approve, sign, or execute an operation.
func (b Binding) EnsureAuthorizable(userID string) error {
	if err := b.Validate(); err != nil {
		return apperrors.Wrap(apperrors.CodeValidationError, "Wallet binding is invalid.", false, true, true, err)
	}
	if b.userID != strings.TrimSpace(userID) {
		return apperrors.New(apperrors.CodeWalletMismatch, "Wallet binding does not match the identity.", false, true, true)
	}
	switch b.status {
	case BindingStatusActive:
		return nil
	case BindingStatusPending:
		return apperrors.New(apperrors.CodeWalletNotBound, "Wallet binding is not active.", false, true, false)
	case BindingStatusRevoked:
		return apperrors.New(apperrors.CodeWalletRevoked, "Wallet binding is revoked.", false, true, true)
	default:
		return apperrors.New(apperrors.CodeInternalError, "An internal error occurred.", false, false, false)
	}
}

// DuplicateKey is the deterministic uniqueness key for a canonical wallet on
// a network. It contains no authorization material.
func (b Binding) DuplicateKey() (string, error) {
	if err := b.Validate(); err != nil {
		return "", err
	}
	return b.chainID + "\x00" + b.network + "\x00" + b.address, nil
}

func (b Binding) BindingID() string             { return b.bindingID }
func (b Binding) Version() uint64               { return b.version }
func (b Binding) OwnerUserID() string           { return b.userID }
func (b Binding) Provider() string              { return b.provider }
func (b Binding) ProviderUserReference() string { return b.providerUserReference }
func (b Binding) WalletID() string              { return b.walletID }
func (b Binding) Address() string               { return b.address }
func (b Binding) ChainID() string               { return b.chainID }
func (b Binding) Network() string               { return b.network }
func (b Binding) Status() BindingStatus         { return b.status }
func (b Binding) VerificationReference() string { return b.verificationReference }
func (b Binding) CreatedAt() time.Time          { return b.createdAt }
func (b Binding) VerifiedAt() time.Time         { return b.verifiedAt }
func (b Binding) RevokedAt() time.Time          { return b.revokedAt }

func validateChainID(chainID string) error {
	if chainID == "" || len(chainID) > maxChainIDLength || chainID[0] == '0' {
		return fmt.Errorf("chain ID must be a canonical positive decimal string")
	}
	for _, character := range chainID {
		if character < '0' || character > '9' {
			return fmt.Errorf("chain ID must be a canonical positive decimal string")
		}
	}
	return nil
}
