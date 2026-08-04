// Package errors defines provider-neutral, publicly safe application errors.
package errors

import (
	stderrors "errors"
)

// Code is a stable public error category defined by the Phase 0 contract.
type Code string

const (
	CodeValidationError         Code = "validation_error"
	CodeAuthenticationRequired  Code = "authentication_required"
	CodeAuthorizationRequired   Code = "authorization_required"
	CodeApprovalRequired        Code = "approval_required"
	CodeIdentityNotFound        Code = "identity_not_found"
	CodeIdentitySuspended       Code = "identity_suspended"
	CodeIdentityRevoked         Code = "identity_revoked"
	CodeWalletNotBound          Code = "wallet_not_bound"
	CodeWalletMismatch          Code = "wallet_mismatch"
	CodeWalletRevoked           Code = "wallet_revoked"
	CodeIntentNotFound          Code = "intent_not_found"
	CodeIntentExpired           Code = "intent_expired"
	CodeIntentMutated           Code = "intent_mutated"
	CodeApprovalNotFound        Code = "approval_not_found"
	CodeApprovalExpired         Code = "approval_expired"
	CodeApprovalRejected        Code = "approval_rejected"
	CodeApprovalAlreadyConsumed Code = "approval_already_consumed"
	CodePolicyNotFound          Code = "policy_not_found"
	CodePolicyInvalid           Code = "policy_invalid"
	CodePolicyDenied            Code = "policy_denied"
	CodePolicyExpired           Code = "policy_expired"
	CodePolicyDisabled          Code = "policy_disabled"
	CodeReviewRequired          Code = "review_required"
	CodeCapabilityNotFound      Code = "capability_not_found"
	CodeCapabilityUnavailable   Code = "capability_unavailable"
	CodeCapabilityConflict      Code = "capability_conflict"
	CodeProviderNotFound        Code = "provider_not_found"
	CodeProviderUnavailable     Code = "provider_unavailable"
	CodeProviderConflict        Code = "provider_conflict"
	CodeContractNotFound        Code = "contract_not_found"
	CodeContractUnavailable     Code = "contract_unavailable"
	CodeContractConflict        Code = "contract_conflict"
	CodeExecutionNotFound       Code = "execution_not_found"
	CodeExecutionInvalid        Code = "execution_invalid"
	CodeExecutionNotAuthorized  Code = "execution_not_authorized"
	CodeExecutionConflict       Code = "execution_conflict"
	CodeExecutionFailed         Code = "execution_failed"
	CodeExecutionRecoverable    Code = "execution_recoverable"
	CodeInternalError           Code = "internal_error"
)

const internalMessage = "An internal error occurred."

// Error carries safe public semantics while retaining an optional internal
// cause that is never included in PublicError.
type Error struct {
	Code               Code
	SafeMessage        string
	Retryable          bool
	UserActionRequired bool
	Terminal           bool
	cause              error
}

// New constructs an application error with no internal cause.
func New(code Code, safeMessage string, retryable, userActionRequired, terminal bool) *Error {
	return &Error{
		Code:               code,
		SafeMessage:        safeMessage,
		Retryable:          retryable,
		UserActionRequired: userActionRequired,
		Terminal:           terminal,
	}
}

// Wrap constructs an application error and retains cause for internal error
// inspection. The cause is never exposed by ToPublic.
func Wrap(code Code, safeMessage string, retryable, userActionRequired, terminal bool, cause error) *Error {
	err := New(code, safeMessage, retryable, userActionRequired, terminal)
	err.cause = cause
	return err
}

func (e *Error) Error() string {
	if e == nil || e.SafeMessage == "" {
		return internalMessage
	}
	return e.SafeMessage
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// PublicError is safe to serialize to an MCP or HTTP client.
type PublicError struct {
	Code               Code   `json:"code"`
	Message            string `json:"message"`
	Retryable          bool   `json:"retryable"`
	UserActionRequired bool   `json:"user_action_required"`
	Terminal           bool   `json:"terminal"`
}

// ToPublic maps known application errors and hides all unknown error details.
func ToPublic(err error) PublicError {
	var appError *Error
	if stderrors.As(err, &appError) && appError.Code.valid() {
		message := appError.SafeMessage
		if message == "" {
			message = internalMessage
		}
		return PublicError{
			Code:               appError.Code,
			Message:            message,
			Retryable:          appError.Retryable,
			UserActionRequired: appError.UserActionRequired,
			Terminal:           appError.Terminal,
		}
	}

	return PublicError{Code: CodeInternalError, Message: internalMessage}
}

func (c Code) valid() bool {
	switch c {
	case CodeValidationError,
		CodeAuthenticationRequired,
		CodeAuthorizationRequired,
		CodeApprovalRequired,
		CodeIdentityNotFound,
		CodeIdentitySuspended,
		CodeIdentityRevoked,
		CodeWalletNotBound,
		CodeWalletMismatch,
		CodeWalletRevoked,
		CodeIntentNotFound,
		CodeIntentExpired,
		CodeIntentMutated,
		CodeApprovalNotFound,
		CodeApprovalExpired,
		CodeApprovalRejected,
		CodeApprovalAlreadyConsumed,
		CodePolicyNotFound,
		CodePolicyInvalid,
		CodePolicyDenied,
		CodePolicyExpired,
		CodePolicyDisabled,
		CodeReviewRequired,
		CodeCapabilityNotFound,
		CodeCapabilityUnavailable,
		CodeCapabilityConflict,
		CodeProviderNotFound,
		CodeProviderUnavailable,
		CodeProviderConflict,
		CodeContractNotFound,
		CodeContractUnavailable,
		CodeContractConflict,
		CodeExecutionNotFound,
		CodeExecutionInvalid,
		CodeExecutionNotAuthorized,
		CodeExecutionConflict,
		CodeExecutionFailed,
		CodeExecutionRecoverable,
		CodeInternalError:
		return true
	default:
		return false
	}
}
