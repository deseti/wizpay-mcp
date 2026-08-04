// Package execution defines provider-neutral execution contracts and lifecycle
// values. It contains no adapter implementation and cannot move funds.
package execution

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	// maxExecutionTextLength bounds generic execution text fields (IDs, etc.).
	maxExecutionTextLength = 256
	// maxAdapterReferenceLength bounds durable adapter references that may embed
	// receipt-observation metadata for restart-safe reorg detection. PostgreSQL
	// adapter_reference is text; this is an application-layer bound only.
	maxAdapterReferenceLength = 512
)

type Status string

const (
	StatusCreated          Status = "CREATED"
	StatusAuthorized       Status = "AUTHORIZED"
	StatusQueued           Status = "QUEUED"
	StatusExecuting        Status = "EXECUTING"
	StatusSubmitted        Status = "SUBMITTED"
	StatusConfirming       Status = "CONFIRMING"
	StatusConfirmed        Status = "CONFIRMED"
	StatusVerified         Status = "VERIFIED"
	StatusCompleted        Status = "COMPLETED"
	StatusFailed           Status = "FAILED"
	StatusRecoveryRequired Status = "RECOVERY_REQUIRED"
	StatusCancelled        Status = "CANCELLED"
)

func (s Status) Valid() bool {
	switch s {
	case StatusCreated, StatusAuthorized, StatusQueued, StatusExecuting, StatusSubmitted,
		StatusConfirming, StatusConfirmed, StatusVerified, StatusCompleted, StatusFailed,
		StatusRecoveryRequired, StatusCancelled:
		return true
	default:
		return false
	}
}

type RecoveryEligibility string

const (
	RecoveryTerminal    RecoveryEligibility = "TERMINAL"
	RecoveryRecoverable RecoveryEligibility = "RECOVERABLE"
)

func (e RecoveryEligibility) Valid() bool { return e == RecoveryTerminal || e == RecoveryRecoverable }

// Failure contains only a stable safe code and deterministic recovery policy.
// It deliberately excludes provider responses and internal error messages.
type Failure struct {
	Code           string
	Eligibility    RecoveryEligibility
	RecoveryTarget Status
}

func (f Failure) validate(failedFrom Status) error {
	if err := validateReasonCode("failure code", f.Code); err != nil {
		return err
	}
	if !f.Eligibility.Valid() {
		return fmt.Errorf("invalid recovery eligibility %q", f.Eligibility)
	}
	if f.Eligibility == RecoveryTerminal {
		if f.RecoveryTarget != "" {
			return fmt.Errorf("terminal failure cannot have a recovery target")
		}
		return nil
	}
	if !validRecoveryTarget(failedFrom, f.RecoveryTarget) {
		return fmt.Errorf("recovery target %s is invalid after failure from %s", f.RecoveryTarget, failedFrom)
	}
	return nil
}

// Recovery records the explicit same-execution reconciliation boundary. It
// does not schedule or perform a retry.
type Recovery struct {
	ReasonCode string
	FromStatus Status
	Target     Status
}

func (r Recovery) validate() error {
	if err := validateReasonCode("recovery reason code", r.ReasonCode); err != nil {
		return err
	}
	if !validRecoveryTarget(r.FromStatus, r.Target) {
		return fmt.Errorf("recovery target %s is invalid from %s", r.Target, r.FromStatus)
	}
	return nil
}

func validRecoveryTarget(from, target Status) bool {
	switch from {
	case StatusQueued, StatusExecuting:
		return target == StatusExecuting
	case StatusSubmitted, StatusConfirming:
		return target == StatusConfirming
	case StatusConfirmed:
		return target == StatusConfirmed
	case StatusVerified:
		return target == StatusVerified
	default:
		return false
	}
}

func validateExecutionText(name, value string) error {
	return validateBoundedText(name, value, maxExecutionTextLength)
}

// validateAdapterReference applies the larger bound reserved for durable
// provider references that embed optional reorg observation metadata.
func validateAdapterReference(value string) error {
	return validateBoundedText("adapter reference", value, maxAdapterReferenceLength)
}

func validateBoundedText(name, value string, max int) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > max {
		return fmt.Errorf("%s exceeds %d characters", name, max)
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s contains control characters", name)
	}
	return nil
}

func validateReasonCode(name, value string) error {
	if len(value) < 2 || len(value) > 64 || value[0] < 'A' || value[0] > 'Z' {
		return fmt.Errorf("%s must be an uppercase safe reason code", name)
	}
	for _, character := range value[1:] {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return fmt.Errorf("%s must be an uppercase safe reason code", name)
		}
	}
	return nil
}
