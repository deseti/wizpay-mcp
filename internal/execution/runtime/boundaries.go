package runtime

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/execution"
)

type VerificationOutcome string

const (
	VerificationPending  VerificationOutcome = "PENDING"
	VerificationVerified VerificationOutcome = "VERIFIED"
	VerificationFailed   VerificationOutcome = "FAILED"
)

type VerificationResult struct {
	Outcome    VerificationOutcome
	Reference  string
	ReasonCode string
	ObservedAt time.Time
}

func (r VerificationResult) Validate() error {
	if r.ObservedAt.IsZero() {
		return fmt.Errorf("verification observation time is required")
	}
	switch r.Outcome {
	case VerificationPending, VerificationVerified:
		if r.Reference == "" || r.ReasonCode != "" {
			return fmt.Errorf("verification reference is required without a failure reason")
		}
	case VerificationFailed:
		if r.ReasonCode == "" {
			return fmt.Errorf("verified failure reason is required")
		}
	default:
		return fmt.Errorf("verification outcome is invalid")
	}
	return nil
}

type Verifier interface {
	Verify(context.Context, execution.Execution, string) (VerificationResult, error)
}

type AdapterFailureKind string

const (
	AdapterFailureTransient AdapterFailureKind = "TRANSIENT"
	AdapterFailurePermanent AdapterFailureKind = "PERMANENT"
)

type AdapterError struct {
	Kind       AdapterFailureKind
	ReasonCode string
}

func NewAdapterError(kind AdapterFailureKind, reasonCode string) error {
	return &AdapterError{Kind: kind, ReasonCode: reasonCode}
}
func (e *AdapterError) Error() string { return "execution adapter failed" }

type VerificationError struct{ ReasonCode string }

func NewVerificationError(reasonCode string) error { return &VerificationError{ReasonCode: reasonCode} }
func (e *VerificationError) Error() string         { return "execution verifier temporarily unavailable" }

func adapterFailure(err error) (*AdapterError, bool) {
	var value *AdapterError
	return value, stderrors.As(err, &value)
}
func verificationFailure(err error) (*VerificationError, bool) {
	var value *VerificationError
	return value, stderrors.As(err, &value)
}
