package providers

import (
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/execution/runtime"
)

// Classification is the provider-neutral outcome taxonomy. Every provider
// interaction must resolve to exactly one of these, and unrecognized provider
// states must fail closed to ClassAmbiguousSubmission rather than be guessed.
type Classification string

const (
	// ClassPreSubmissionValidationFailure means the request was rejected before
	// anything could reach the provider's financial surface.
	ClassPreSubmissionValidationFailure Classification = "PRE_SUBMISSION_VALIDATION_FAILURE"
	// ClassUserAuthorizationRequired means the operation is waiting on the
	// user's own signing authorization. It is never a provider failure, and it
	// must never be resolved by backend orchestration.
	ClassUserAuthorizationRequired Classification = "USER_AUTHORIZATION_REQUIRED"
	// ClassTransientProviderError means the provider was momentarily
	// unavailable. It is always recoverable and never terminal.
	ClassTransientProviderError Classification = "TRANSIENT_PROVIDER_ERROR"
	// ClassPermanentProviderRejection means the provider definitively refused
	// the operation and no submission occurred.
	ClassPermanentProviderRejection Classification = "PERMANENT_PROVIDER_REJECTION"
	// ClassAmbiguousSubmission means submission may or may not have occurred.
	// It is reconciliation-only: it must never cause a resubmission.
	ClassAmbiguousSubmission Classification = "AMBIGUOUS_SUBMISSION"
	// ClassSubmittedPending means the provider accepted the operation and an
	// outcome is still pending. It is an observation, not success.
	ClassSubmittedPending Classification = "SUBMITTED_PENDING"
	// ClassConfirmedOnchainFailed means an on-chain receipt confirmed the
	// transaction executed and failed. Only a receipt may produce this.
	ClassConfirmedOnchainFailed Classification = "CONFIRMED_ONCHAIN_FAILED"
	// ClassVerifiedSuccess means an on-chain receipt confirmed successful
	// execution. A provider status alone can never produce this.
	ClassVerifiedSuccess Classification = "VERIFIED_SUCCESS"
)

func (c Classification) Valid() bool {
	switch c {
	case ClassPreSubmissionValidationFailure, ClassUserAuthorizationRequired,
		ClassTransientProviderError, ClassPermanentProviderRejection,
		ClassAmbiguousSubmission, ClassSubmittedPending,
		ClassConfirmedOnchainFailed, ClassVerifiedSuccess:
		return true
	default:
		return false
	}
}

// ReconciliationOnly reports whether an outcome forbids any further submission
// attempt. Once external submission may have occurred, the only safe action is
// to reconcile provider and chain state.
func (c Classification) ReconciliationOnly() bool {
	switch c {
	case ClassPreSubmissionValidationFailure, ClassPermanentProviderRejection:
		return false
	default:
		return true
	}
}

// Terminal reports whether an outcome ends the execution with no recovery.
func (c Classification) Terminal() bool {
	switch c {
	case ClassPreSubmissionValidationFailure, ClassPermanentProviderRejection, ClassConfirmedOnchainFailed:
		return true
	default:
		return false
	}
}

// Outcome is one classified provider observation. It carries only safe
// reference material: never a raw response body, credential, or payload.
type Outcome struct {
	Class      Classification
	ReasonCode string
	Reference  Reference
	ObservedAt time.Time
	// HasReference distinguishes "no provider identifier is known yet" from a
	// zero-valued reference, so a missing reference can never be encoded.
	HasReference bool
}

func (o Outcome) Validate() error {
	if !o.Class.Valid() {
		return fmt.Errorf("provider outcome classification is invalid")
	}
	if o.ObservedAt.IsZero() {
		return fmt.Errorf("provider outcome observation time is required")
	}
	switch o.Class {
	case ClassSubmittedPending, ClassVerifiedSuccess:
		// These map onto statuses that forbid an error code.
		if o.ReasonCode != "" {
			return fmt.Errorf("provider outcome %s cannot carry a reason code", o.Class)
		}
		if !o.HasReference {
			return fmt.Errorf("provider outcome %s requires a provider reference", o.Class)
		}
	default:
		if !validReasonCode(o.ReasonCode) {
			return fmt.Errorf("provider outcome %s requires a safe reason code", o.Class)
		}
	}
	if o.HasReference {
		if err := o.Reference.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// AdapterResult converts a classified outcome into the Phase 9 adapter
// contract: either an execution.Result the runtime can persist, or a typed
// runtime adapter error.
//
// The runtime marks submission-started before it calls Execute, so every
// recoverable path returned here re-enters reconciliation through GetStatus
// rather than a second Execute. Nothing in this mapping can produce VERIFIED:
// only an on-chain receipt, through the verifier, may do that.
func (o Outcome) AdapterResult(executionID string) (execution.Result, error) {
	if err := o.Validate(); err != nil {
		return execution.Result{}, err
	}
	params := execution.ResultParams{
		ExecutionID: executionID,
		// The Phase 9 result contract compares against the request contract
		// version, not the execution revision.
		ExecutionVersion: execution.RequestVersion,
		ObservedAt:       o.ObservedAt,
	}
	reference, err := o.encodedReference()
	if err != nil {
		return execution.Result{}, err
	}
	switch o.Class {
	case ClassPreSubmissionValidationFailure, ClassPermanentProviderRejection:
		return execution.Result{}, runtime.NewAdapterError(runtime.AdapterFailurePermanent, o.ReasonCode)
	case ClassTransientProviderError:
		return execution.Result{}, runtime.NewAdapterError(runtime.AdapterFailureTransient, o.ReasonCode)
	case ClassUserAuthorizationRequired, ClassAmbiguousSubmission:
		params.Status = execution.StatusRecoveryRequired
		params.ErrorCode = o.ReasonCode
		params.AdapterReference = reference
	case ClassSubmittedPending:
		params.Status = execution.StatusSubmitted
		params.AdapterReference = reference
	case ClassConfirmedOnchainFailed:
		params.Status = execution.StatusFailed
		params.ErrorCode = o.ReasonCode
		params.AdapterReference = reference
	case ClassVerifiedSuccess:
		// A provider adapter never reports verified success. On-chain evidence
		// is the verifier's responsibility, so this degrades to an observation.
		params.Status = execution.StatusConfirming
		params.AdapterReference = reference
	default:
		return execution.Result{}, fmt.Errorf("provider outcome classification is invalid")
	}
	return execution.NewResult(params)
}

// VerificationResult converts a classified outcome into the Phase 9 verifier
// contract. Only ClassVerifiedSuccess yields VERIFIED, and only
// ClassConfirmedOnchainFailed yields FAILED; every other observation leaves the
// execution pending rather than asserting an unproven outcome.
func (o Outcome) VerificationResult() (runtime.VerificationResult, error) {
	if err := o.Validate(); err != nil {
		return runtime.VerificationResult{}, err
	}
	reference, err := o.encodedReference()
	if err != nil {
		return runtime.VerificationResult{}, err
	}
	switch o.Class {
	case ClassVerifiedSuccess:
		return runtime.VerificationResult{Outcome: runtime.VerificationVerified, Reference: reference, ObservedAt: o.ObservedAt}, nil
	case ClassConfirmedOnchainFailed:
		return runtime.VerificationResult{Outcome: runtime.VerificationFailed, ReasonCode: o.ReasonCode, ObservedAt: o.ObservedAt}, nil
	case ClassTransientProviderError:
		// Unavailability is not evidence. Leave the execution untouched.
		return runtime.VerificationResult{}, runtime.NewVerificationError(o.ReasonCode)
	default:
		if reference == "" {
			return runtime.VerificationResult{}, runtime.NewVerificationError(o.ReasonCode)
		}
		return runtime.VerificationResult{Outcome: runtime.VerificationPending, Reference: reference, ObservedAt: o.ObservedAt}, nil
	}
}

func (o Outcome) encodedReference() (string, error) {
	if !o.HasReference {
		return "", nil
	}
	return o.Reference.Encode()
}

// validReasonCode mirrors the Phase 5 safe reason-code contract: an uppercase
// stable token that is safe to persist, log, and return to a caller.
func validReasonCode(value string) bool {
	if len(value) < 2 || len(value) > 64 {
		return false
	}
	if value[0] < 'A' || value[0] > 'Z' {
		return false
	}
	for _, character := range value[1:] {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}
