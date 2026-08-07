package approvals

import (
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

type Status string

const (
	StatusPending                       Status = "PENDING"
	StatusApproved                      Status = "APPROVED"
	StatusReadyForExecutionConfirmation Status = "READY_FOR_EXECUTION_CONFIRMATION"
	StatusRejected                      Status = "REJECTED"
	StatusExpired                       Status = "EXPIRED"
	StatusConsumed                      Status = "CONSUMED"
)

func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusApproved, StatusReadyForExecutionConfirmation, StatusRejected, StatusExpired, StatusConsumed:
		return true
	default:
		return false
	}
}

func (s Status) Terminal() bool {
	return s == StatusRejected || s == StatusExpired || s == StatusConsumed
}

type Decision string

const (
	DecisionPending  Decision = "PENDING"
	DecisionApproved Decision = "APPROVED"
	DecisionRejected Decision = "REJECTED"
)

func (d Decision) Valid() bool {
	return d == DecisionPending || d == DecisionApproved || d == DecisionRejected
}

func (a Approval) Approve(at time.Time) (Approval, error) {
	if err := a.Validate(); err != nil {
		return Approval{}, err
	}
	if a.status == StatusApproved {
		return a, nil
	}
	if a.status != StatusPending {
		return Approval{}, invalidTransition()
	}
	if err := a.validateDecisionTime(at); err != nil {
		return Approval{}, err
	}
	next := a
	next.status, next.decision, next.decidedAt = StatusApproved, DecisionApproved, at.UTC()
	next.lifecycleRevision++
	return next, next.Validate()
}

// ReadyForExecutionConfirmation records the authenticated handoff to a
// future execution preparation boundary. It never signs or submits anything.
func (a Approval) ReadyForExecutionConfirmation(at time.Time) (Approval, error) {
	if err := a.Validate(); err != nil {
		return Approval{}, err
	}
	if a.status == StatusReadyForExecutionConfirmation {
		return a, nil
	}
	if a.status != StatusApproved {
		return Approval{}, invalidTransition()
	}
	if at.IsZero() || !at.Before(a.expiresAt) {
		return Approval{}, apperrors.New(apperrors.CodeApprovalExpired, "Approval has expired.", false, true, true)
	}
	next := a
	next.status = StatusReadyForExecutionConfirmation
	next.lifecycleRevision++
	return next, next.Validate()
}

func (a Approval) Reject(at time.Time) (Approval, error) {
	if err := a.Validate(); err != nil {
		return Approval{}, err
	}
	if a.status == StatusRejected {
		return a, nil
	}
	if a.status != StatusPending {
		return Approval{}, invalidTransition()
	}
	if err := a.validateDecisionTime(at); err != nil {
		return Approval{}, err
	}
	next := a
	next.status, next.decision, next.decidedAt = StatusRejected, DecisionRejected, at.UTC()
	next.lifecycleRevision++
	return next, next.Validate()
}

func (a Approval) Expire(at time.Time) (Approval, error) {
	if err := a.Validate(); err != nil {
		return Approval{}, err
	}
	if a.status == StatusExpired {
		return a, nil
	}
	if a.status != StatusPending && a.status != StatusApproved && a.status != StatusReadyForExecutionConfirmation {
		return Approval{}, invalidTransition()
	}
	if at.IsZero() || at.Before(a.expiresAt) {
		return Approval{}, apperrors.New(apperrors.CodeValidationError, "Approval has not reached its expiration time.", false, true, false)
	}
	next := a
	next.status = StatusExpired
	next.lifecycleRevision++
	return next, next.Validate()
}

// Consume reserves an approved artifact for its deterministic logical
// operation. It does not create or execute a financial transaction.
func (a Approval) Consume(at time.Time, operation intents.OperationIdentity) (Approval, error) {
	if err := a.Validate(); err != nil {
		return Approval{}, err
	}
	if a.status == StatusConsumed {
		if a.operationKey == operation.OperationKey() && a.operationVersion == operation.Version() {
			return a, nil
		}
		return Approval{}, apperrors.New(apperrors.CodeApprovalAlreadyConsumed, "Approval has already been consumed.", false, true, true)
	}
	if a.status != StatusApproved && a.status != StatusReadyForExecutionConfirmation {
		return Approval{}, invalidTransition()
	}
	if at.IsZero() || at.Before(a.createdAt) {
		return Approval{}, apperrors.New(apperrors.CodeValidationError, "Approval consumption time is invalid.", false, true, true)
	}
	if !at.Before(a.expiresAt) {
		return Approval{}, apperrors.New(apperrors.CodeApprovalExpired, "Approval has expired.", false, true, true)
	}
	if a.intentID != operation.IntentID() || a.intentVersion != operation.IntentVersion() || a.intentDigest != operation.IntentDigest() || operation.OperationKey() == "" {
		return Approval{}, apperrors.New(apperrors.CodeApprovalRequired, "Approval does not match the logical operation.", false, true, true)
	}
	next := a
	next.status, next.consumedAt, next.operationKey, next.operationVersion = StatusConsumed, at.UTC(), operation.OperationKey(), operation.Version()
	next.lifecycleRevision++
	return next, next.Validate()
}

func (a Approval) validateDecisionTime(at time.Time) error {
	if at.IsZero() || at.Before(a.createdAt) {
		return apperrors.New(apperrors.CodeValidationError, "Approval decision time is invalid.", false, true, true)
	}
	if !at.Before(a.expiresAt) {
		return apperrors.New(apperrors.CodeApprovalExpired, "Approval has expired.", false, true, true)
	}
	return nil
}

func invalidTransition() error {
	return apperrors.New(apperrors.CodeValidationError, "Approval lifecycle transition is invalid.", false, true, true)
}
