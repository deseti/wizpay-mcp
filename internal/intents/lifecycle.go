package intents

import (
	"fmt"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

// Status contains only states before a future execution engine takes custody.
type Status string

const (
	StatusDraft             Status = "DRAFT"
	StatusCreated           Status = "CREATED"
	StatusApprovalRequired  Status = "APPROVAL_REQUIRED"
	StatusApproved          Status = "APPROVED"
	StatusReadyForExecution Status = "READY_FOR_EXECUTION"
	StatusExpired           Status = "EXPIRED"
	StatusCancelled         Status = "CANCELLED"
)

func (s Status) Valid() bool {
	switch s {
	case StatusDraft, StatusCreated, StatusApprovalRequired, StatusApproved,
		StatusReadyForExecution, StatusExpired, StatusCancelled:
		return true
	default:
		return false
	}
}

func (s Status) Terminal() bool {
	return s == StatusExpired || s == StatusCancelled
}

// Transition returns a new state value. Repeated delivery is idempotent.
// CREATED freezes material fields and assigns the deterministic digest.
func (i Intent) Transition(next Status, at time.Time) (Intent, error) {
	if err := i.Validate(); err != nil {
		return Intent{}, err
	}
	if next == i.status {
		return i, nil
	}
	if next == StatusApproved {
		return Intent{}, apperrors.New(apperrors.CodeApprovalRequired, "Explicit matching approval is required.", false, true, false)
	}
	if at.IsZero() || at.Before(i.params.CreatedAt) {
		return Intent{}, apperrors.New(apperrors.CodeValidationError, "Intent transition time is invalid.", false, true, true)
	}
	if err := validTransition(i.status, next); err != nil {
		return Intent{}, apperrors.Wrap(apperrors.CodeValidationError, "Intent lifecycle transition is invalid.", false, true, true, err)
	}
	if next == StatusExpired {
		if at.Before(i.params.ExpiresAt) {
			return Intent{}, apperrors.New(apperrors.CodeValidationError, "Intent has not reached its expiration time.", false, true, false)
		}
	} else if !at.Before(i.params.ExpiresAt) {
		return Intent{}, apperrors.New(apperrors.CodeIntentExpired, "Intent has expired.", false, true, true)
	}

	nextIntent := i
	if next == StatusCreated {
		canonical, err := canonicalMaterial(i.params)
		if err != nil {
			return Intent{}, err
		}
		nextIntent.digest = digestBytes(canonical)
	}
	nextIntent.status = next
	if err := nextIntent.Validate(); err != nil {
		return Intent{}, err
	}
	return nextIntent, nil
}

// ApprovalArtifact is the narrow inward-facing contract implemented by an
// explicit approval value. It avoids a package cycle while keeping approval as
// the only route into APPROVED.
type ApprovalArtifact interface {
	EnsureAuthorizes(Intent, time.Time) error
}

// Approve advances an APPROVAL_REQUIRED intent only after the supplied
// artifact proves an exact, currently effective binding.
func (i Intent) Approve(artifact ApprovalArtifact, at time.Time) (Intent, error) {
	if err := i.Validate(); err != nil {
		return Intent{}, err
	}
	if i.status != StatusApprovalRequired && i.status != StatusApproved {
		return Intent{}, apperrors.New(apperrors.CodeValidationError, "Intent is not awaiting approval.", false, true, true)
	}
	if artifact == nil {
		return Intent{}, apperrors.New(apperrors.CodeApprovalRequired, "Explicit matching approval is required.", false, true, false)
	}
	if at.IsZero() || at.Before(i.params.CreatedAt) {
		return Intent{}, apperrors.New(apperrors.CodeValidationError, "Intent approval time is invalid.", false, true, true)
	}
	if !at.Before(i.params.ExpiresAt) {
		return Intent{}, apperrors.New(apperrors.CodeIntentExpired, "Intent has expired.", false, true, true)
	}
	if err := artifact.EnsureAuthorizes(i, at); err != nil {
		return Intent{}, err
	}
	if i.status == StatusApproved {
		return i, nil
	}
	next := i
	next.status = StatusApproved
	return next, next.Validate()
}

func validTransition(current, next Status) error {
	allowed := false
	switch current {
	case StatusDraft:
		allowed = next == StatusCreated
	case StatusCreated:
		allowed = next == StatusApprovalRequired || next == StatusCancelled || next == StatusExpired
	case StatusApprovalRequired:
		allowed = next == StatusCancelled || next == StatusExpired
	case StatusApproved:
		allowed = next == StatusReadyForExecution || next == StatusCancelled || next == StatusExpired
	case StatusReadyForExecution:
		allowed = next == StatusCancelled || next == StatusExpired
	}
	if !allowed {
		return fmt.Errorf("transition from %s to %s is not allowed", current, next)
	}
	return nil
}
