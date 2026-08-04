package execution

import (
	"fmt"
	"strings"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

type ResultParams struct {
	ExecutionID      string
	ExecutionVersion uint64
	Status           Status
	AdapterReference string
	ErrorCode        string
	ObservedAt       time.Time
}

// EnsureMatches prevents an adapter observation from being applied to another
// logical execution or request contract version.
func (r Result) EnsureMatches(request Request) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if r.executionID != request.ExecutionID() || r.executionVersion != request.Version() {
		return apperrors.New(apperrors.CodeExecutionConflict, "Execution result does not match the request.", false, true, true)
	}
	return nil
}

// Result is a provider-neutral observation. It intentionally has no
// transaction hash, receipt, confirmation object, or raw response.
type Result struct {
	executionID      string
	executionVersion uint64
	status           Status
	adapterReference string
	errorCode        string
	observedAt       time.Time
}

func NewResult(params ResultParams) (Result, error) {
	result := Result{
		executionID: strings.TrimSpace(params.ExecutionID), executionVersion: params.ExecutionVersion,
		status: params.Status, adapterReference: strings.TrimSpace(params.AdapterReference),
		errorCode: strings.TrimSpace(params.ErrorCode), observedAt: params.ObservedAt.UTC(),
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (r Result) Validate() error {
	if err := validateExecutionText("result execution ID", r.executionID); err != nil {
		return invalidExecution(err)
	}
	if r.executionVersion == 0 || r.observedAt.IsZero() {
		return invalidExecution(fmt.Errorf("execution result metadata is invalid"))
	}
	switch r.status {
	case StatusSubmitted, StatusConfirming, StatusConfirmed, StatusVerified, StatusCompleted:
		if r.adapterReference == "" {
			return invalidExecution(fmt.Errorf("execution result reference is required"))
		}
		if r.errorCode != "" {
			return invalidExecution(fmt.Errorf("successful execution result cannot have an error code"))
		}
	case StatusFailed, StatusRecoveryRequired:
		if r.errorCode == "" {
			return invalidExecution(fmt.Errorf("failed or recoverable result requires an error code"))
		}
	default:
		return invalidExecution(fmt.Errorf("status %s is not an adapter result state", r.status))
	}
	if r.adapterReference != "" {
		if err := validateAdapterReference(r.adapterReference); err != nil {
			return invalidExecution(err)
		}
	}
	if r.errorCode != "" {
		if err := validateReasonCode("result error code", r.errorCode); err != nil {
			return invalidExecution(err)
		}
	}
	return nil
}

func (r Result) ExecutionID() string      { return r.executionID }
func (r Result) ExecutionVersion() uint64 { return r.executionVersion }
func (r Result) Status() Status           { return r.status }
func (r Result) AdapterReference() string { return r.adapterReference }
func (r Result) ErrorCode() string        { return r.errorCode }
func (r Result) ObservedAt() time.Time    { return r.observedAt }
