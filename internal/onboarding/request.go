// Package onboarding defines owner-only enrollment administration. Pending
// records confer no authority. Activation consumes trusted server-verified wallet
// proof and an independent delegated owner decision about application identity.
package onboarding

import (
	"errors"
	"regexp"
	"time"

	"github.com/deseti/wizpay-mcp/internal/siwe"
)

var ErrDenied = errors.New("onboarding administration denied")
var proofIdentifier = regexp.MustCompile(`^[0-9a-f]{64}$`)
var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

// References identify protected owner review records, never their contents.
// No signature, identity document, token or signing key is accepted here.
type Request struct {
	Action          string    `json:"action"`
	RequestID       string    `json:"request_id"`
	TenantID        string    `json:"tenant_id"`
	UserID          string    `json:"user_id"`
	BindingID       string    `json:"binding_id"`
	Issuer          string    `json:"issuer"`
	Subject         string    `json:"subject"`
	Address         string    `json:"address"`
	ReviewReference string    `json:"review_reference"`
	VerificationID  string    `json:"verification_id"`
	ExpiresAt       time.Time `json:"expires_at"`
}

func (r Request) Validate(now time.Time) error {
	if now.IsZero() || !r.ExpiresAt.After(now) || r.ExpiresAt.After(now.Add(15*time.Minute)) {
		return ErrDenied
	}
	for _, value := range []string{r.RequestID, r.TenantID, r.ReviewReference} {
		if !identifier.MatchString(value) {
			return ErrDenied
		}
	}
	if r.Action != "ACTIVATE" && r.VerificationID != "" {
		return ErrDenied
	}
	switch r.Action {
	case "ACTIVATE":
		if !identifier.MatchString(r.UserID) || !identifier.MatchString(r.BindingID) || !proofIdentifier.MatchString(r.VerificationID) || r.Issuer != "" || r.Subject != "" || r.Address != "" {
			return ErrDenied
		}
	case "STAGE":
		for _, value := range []string{r.UserID, r.BindingID, r.Issuer, r.Subject} {
			if !identifier.MatchString(value) {
				return ErrDenied
			}
		}
		a, err := siwe.Address(r.Address)
		if err != nil || a != r.Address {
			return ErrDenied
		}
	case "REVOKE_TENANT", "REVOKE_IDENTITY", "REVOKE_BINDING":
		if r.Issuer != "" || r.Subject != "" || r.Address != "" {
			return ErrDenied
		}
		if r.Action == "REVOKE_TENANT" {
			if r.UserID != "" || r.BindingID != "" {
				return ErrDenied
			}
		} else if !identifier.MatchString(r.UserID) {
			return ErrDenied
		}
		if r.Action == "REVOKE_BINDING" {
			if !identifier.MatchString(r.BindingID) {
				return ErrDenied
			}
		} else if r.BindingID != "" {
			return ErrDenied
		}
	default:
		return ErrDenied
	}
	return nil
}
