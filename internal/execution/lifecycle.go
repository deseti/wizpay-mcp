package execution

import (
	"fmt"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

// Execution is an immutable lifecycle record around one validated Request.
// State changes return a new value and never invoke an adapter.
type Execution struct {
	request    Request
	status     Status
	revision   uint64
	createdAt  time.Time
	updatedAt  time.Time
	failure    Failure
	failedFrom Status
	recovery   Recovery
}

func New(request Request) (Execution, error) {
	if err := request.Validate(); err != nil {
		return Execution{}, err
	}
	execution := Execution{request: request, status: StatusCreated, revision: 1, createdAt: request.CreatedAt(), updatedAt: request.CreatedAt()}
	return execution, execution.Validate()
}

func (e Execution) Validate() error {
	if err := e.request.Validate(); err != nil {
		return err
	}
	if !e.status.Valid() {
		return invalidExecution(fmt.Errorf("invalid execution status %q", e.status))
	}
	if e.revision == 0 || e.createdAt.IsZero() || e.updatedAt.IsZero() || e.updatedAt.Before(e.createdAt) {
		return invalidExecution(fmt.Errorf("execution lifecycle metadata is invalid"))
	}
	if e.status == StatusFailed {
		if !e.failedFrom.Valid() || e.failedFrom == StatusFailed || e.failure.Code == "" {
			return invalidExecution(fmt.Errorf("failed execution metadata is invalid"))
		}
		if err := e.failure.validate(e.failedFrom); err != nil {
			return invalidExecution(err)
		}
	}
	if e.status == StatusRecoveryRequired {
		if err := e.recovery.validate(); err != nil {
			return invalidExecution(err)
		}
	}
	return nil
}

// Transition advances ordinary lifecycle states. FAILED and
// RECOVERY_REQUIRED require Fail or RequireRecovery so safe metadata cannot be
// omitted. Repeated delivery is an idempotent no-op.
func (e Execution) Transition(next Status, at time.Time) (Execution, error) {
	if err := e.Validate(); err != nil {
		return Execution{}, err
	}
	if next == e.status {
		return e, nil
	}
	if next == StatusFailed || next == StatusRecoveryRequired {
		return Execution{}, invalidExecution(fmt.Errorf("failure and recovery transitions require explicit metadata"))
	}
	if err := e.validateTransitionTime(at); err != nil {
		return Execution{}, err
	}
	if !validTransition(e.status, next) {
		return Execution{}, invalidExecution(fmt.Errorf("execution transition %s -> %s is not allowed", e.status, next))
	}
	return e.advance(next, at)
}

func validTransition(current, next Status) bool {
	switch current {
	case StatusCreated:
		return next == StatusAuthorized || next == StatusCancelled
	case StatusAuthorized:
		return next == StatusQueued || next == StatusCancelled
	case StatusQueued:
		return next == StatusExecuting
	case StatusExecuting:
		return next == StatusSubmitted || next == StatusConfirming
	case StatusSubmitted:
		return next == StatusConfirming
	case StatusConfirming:
		return next == StatusConfirmed
	case StatusConfirmed:
		return next == StatusVerified
	case StatusVerified:
		return next == StatusCompleted
	default:
		return false
	}
}

// Fail records a proven failure. Only a failure explicitly marked recoverable
// with a valid same-execution target may enter recovery later.
func (e Execution) Fail(failure Failure, at time.Time) (Execution, error) {
	if err := e.Validate(); err != nil {
		return Execution{}, err
	}
	if e.status == StatusFailed {
		if e.failure == failure {
			return e, nil
		}
		return Execution{}, apperrors.New(apperrors.CodeExecutionConflict, "Execution failure record conflicts with existing state.", false, true, true)
	}
	if err := e.validateTransitionTime(at); err != nil {
		return Execution{}, err
	}
	switch e.status {
	case StatusExecuting, StatusSubmitted, StatusConfirming, StatusConfirmed:
	default:
		return Execution{}, invalidExecution(fmt.Errorf("execution cannot fail from %s", e.status))
	}
	if err := failure.validate(e.status); err != nil {
		return Execution{}, invalidExecution(err)
	}
	next := e
	next.failure, next.failedFrom = failure, e.status
	return next.advance(StatusFailed, at)
}

// RequireRecovery records ambiguity for the same execution without calling an
// adapter or scheduling work. The safe resume target is derived from state.
func (e Execution) RequireRecovery(reasonCode string, at time.Time) (Execution, error) {
	if err := e.Validate(); err != nil {
		return Execution{}, err
	}
	if e.status == StatusRecoveryRequired {
		if e.recovery.ReasonCode == reasonCode {
			return e, nil
		}
		return Execution{}, apperrors.New(apperrors.CodeExecutionConflict, "Recovery record conflicts with existing state.", false, true, true)
	}
	if err := e.validateTransitionTime(at); err != nil {
		return Execution{}, err
	}
	target := recoveryTarget(e.status)
	recovery := Recovery{ReasonCode: reasonCode, FromStatus: e.status, Target: target}
	if err := recovery.validate(); err != nil {
		return Execution{}, invalidExecution(err)
	}
	next := e
	next.recovery = recovery
	return next.advance(StatusRecoveryRequired, at)
}

// Recover converts an explicitly recoverable FAILED state into the same
// execution's RECOVERY_REQUIRED state. It performs no retry.
func (e Execution) Recover(at time.Time) (Execution, error) {
	if err := e.Validate(); err != nil {
		return Execution{}, err
	}
	if e.status != StatusFailed {
		return Execution{}, invalidExecution(fmt.Errorf("only failed execution can use failure recovery"))
	}
	if e.failure.Eligibility != RecoveryRecoverable {
		return Execution{}, apperrors.New(apperrors.CodeExecutionFailed, "Execution failure is terminal.", false, true, true)
	}
	if err := e.validateTransitionTime(at); err != nil {
		return Execution{}, err
	}
	next := e
	next.recovery = Recovery{ReasonCode: e.failure.Code, FromStatus: e.failedFrom, Target: e.failure.RecoveryTarget}
	return next.advance(StatusRecoveryRequired, at)
}

// Resume leaves recovery only at its prevalidated checkpoint. It does not run
// the resumed work.
func (e Execution) Resume(at time.Time) (Execution, error) {
	if err := e.Validate(); err != nil {
		return Execution{}, err
	}
	if e.status != StatusRecoveryRequired {
		return Execution{}, invalidExecution(fmt.Errorf("execution is not awaiting recovery"))
	}
	if err := e.validateTransitionTime(at); err != nil {
		return Execution{}, err
	}
	return e.advance(e.recovery.Target, at)
}

func recoveryTarget(status Status) Status {
	switch status {
	case StatusQueued, StatusExecuting:
		return StatusExecuting
	case StatusSubmitted, StatusConfirming:
		return StatusConfirming
	case StatusConfirmed:
		return StatusConfirmed
	case StatusVerified:
		return StatusVerified
	default:
		return ""
	}
}

func (e Execution) validateTransitionTime(at time.Time) error {
	if at.IsZero() || at.Before(e.updatedAt) {
		return invalidExecution(fmt.Errorf("execution transition time is invalid"))
	}
	if e.revision == ^uint64(0) {
		return invalidExecution(fmt.Errorf("execution revision cannot advance"))
	}
	return nil
}

func (e Execution) advance(status Status, at time.Time) (Execution, error) {
	next := e
	next.status, next.updatedAt, next.revision = status, at.UTC(), e.revision+1
	if err := next.Validate(); err != nil {
		return Execution{}, err
	}
	return next, nil
}

func (e Execution) Terminal() bool {
	return e.status == StatusCompleted || e.status == StatusCancelled ||
		(e.status == StatusFailed && e.failure.Eligibility == RecoveryTerminal)
}

func (e Execution) Request() Request           { return e.request }
func (e Execution) ExecutionID() string        { return e.request.ExecutionID() }
func (e Execution) Status() Status             { return e.status }
func (e Execution) Revision() uint64           { return e.revision }
func (e Execution) CreatedAt() time.Time       { return e.createdAt }
func (e Execution) UpdatedAt() time.Time       { return e.updatedAt }
func (e Execution) Failure() (Failure, bool)   { return e.failure, e.failure.Code != "" }
func (e Execution) Recovery() (Recovery, bool) { return e.recovery, e.recovery.ReasonCode != "" }
