// Package auth defines transport-neutral identity and authorization contracts.
// It does not authenticate users or integrate with identity providers.
package auth

import (
	"fmt"
	"strings"
	"unicode"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

const maxIdentityFieldLength = 256

// IdentityStatus represents application identity lifecycle state independently
// from wallet-binding state.
type IdentityStatus string

const (
	IdentityStatusUnknown   IdentityStatus = "UNKNOWN"
	IdentityStatusActive    IdentityStatus = "ACTIVE"
	IdentityStatusSuspended IdentityStatus = "SUSPENDED"
	IdentityStatusRevoked   IdentityStatus = "REVOKED"
)

// Identity is a validated, provider-neutral application user identity.
type Identity struct {
	userID   string
	provider string
	status   IdentityStatus
}

// NewIdentity validates and creates an identity value.
func NewIdentity(userID, provider string, status IdentityStatus) (Identity, error) {
	identity := Identity{
		userID:   strings.TrimSpace(userID),
		provider: strings.TrimSpace(provider),
		status:   status,
	}
	if err := identity.Validate(); err != nil {
		return Identity{}, err
	}
	return identity, nil
}

// Validate checks structural identity invariants without authenticating it.
func (i Identity) Validate() error {
	if err := validateIdentityField("user ID", i.userID); err != nil {
		return err
	}
	if err := validateIdentityField("identity provider", i.provider); err != nil {
		return err
	}
	if !i.status.Valid() {
		return fmt.Errorf("invalid identity status %q", i.status)
	}
	return nil
}

// Transition returns a new identity in next state. Repeated state delivery is
// idempotent, and revoked identities are terminal.
func (i Identity) Transition(next IdentityStatus) (Identity, error) {
	if err := i.Validate(); err != nil {
		return Identity{}, err
	}
	if !next.Valid() {
		return Identity{}, fmt.Errorf("invalid identity status %q", next)
	}
	if next == i.status {
		return i, nil
	}

	allowed := false
	switch i.status {
	case IdentityStatusUnknown:
		allowed = next == IdentityStatusActive || next == IdentityStatusSuspended || next == IdentityStatusRevoked
	case IdentityStatusActive:
		allowed = next == IdentityStatusSuspended || next == IdentityStatusRevoked
	case IdentityStatusSuspended:
		allowed = next == IdentityStatusActive || next == IdentityStatusRevoked
	case IdentityStatusRevoked:
		allowed = false
	}
	if !allowed {
		return Identity{}, fmt.Errorf("identity transition %s -> %s is not allowed", i.status, next)
	}

	i.status = next
	return i, nil
}

// EnsureAuthorizable enforces identity lifecycle eligibility only. It does not
// authenticate the identity or approve an operation.
func (i Identity) EnsureAuthorizable() error {
	if err := i.Validate(); err != nil {
		return apperrors.Wrap(apperrors.CodeValidationError, "Identity is invalid.", false, true, true, err)
	}

	switch i.status {
	case IdentityStatusActive:
		return nil
	case IdentityStatusUnknown:
		return apperrors.New(apperrors.CodeIdentityNotFound, "Identity is not active.", false, true, false)
	case IdentityStatusSuspended:
		return apperrors.New(apperrors.CodeIdentitySuspended, "Identity is suspended.", false, true, false)
	case IdentityStatusRevoked:
		return apperrors.New(apperrors.CodeIdentityRevoked, "Identity is revoked.", false, true, true)
	default:
		return apperrors.New(apperrors.CodeInternalError, "An internal error occurred.", false, false, false)
	}
}

func (i Identity) UserID() string         { return i.userID }
func (i Identity) Provider() string       { return i.provider }
func (i Identity) Status() IdentityStatus { return i.status }
func (s IdentityStatus) Valid() bool {
	switch s {
	case IdentityStatusUnknown, IdentityStatusActive, IdentityStatusSuspended, IdentityStatusRevoked:
		return true
	default:
		return false
	}
}

func validateIdentityField(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > maxIdentityFieldLength {
		return fmt.Errorf("%s exceeds %d characters", name, maxIdentityFieldLength)
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s contains control characters", name)
	}
	return nil
}
