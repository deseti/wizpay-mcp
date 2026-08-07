package wiring

import (
	"context"
	"fmt"

	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/execution/runtime"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/payroll"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/swap"
)

// ComposedVerifier adds the typed Payroll/Swap domain gate after generic provider
// reconciliation.
//
// The generic verifier owns the single chain receipt read and returns the
// exact receipt observation used for generic classification. Domain verifiers
// consume that observation directly and perform no I/O.
type ComposedVerifier struct {
	provider *providers.Verifier
	intents  storage.IntentRepository
	payroll  *payroll.Planner
	swap     *swap.Planner
	payrollV *payroll.Verifier
	swapV    *swap.Verifier
}

func NewComposedVerifier(provider *providers.Verifier, repository storage.IntentRepository, payrollPlanner *payroll.Planner, swapPlanner *swap.Planner, payrollVerifier *payroll.Verifier, swapVerifier *swap.Verifier) (*ComposedVerifier, error) {
	if provider == nil || repository == nil || payrollPlanner == nil || swapPlanner == nil || payrollVerifier == nil || swapVerifier == nil {
		return nil, fmt.Errorf("composed verifier dependencies are required")
	}
	return &ComposedVerifier{
		provider: provider, intents: repository,
		payroll: payrollPlanner, swap: swapPlanner,
		payrollV: payrollVerifier, swapV: swapVerifier,
	}, nil
}

var _ runtime.Verifier = (*ComposedVerifier)(nil)

// Verify preserves generic pending and failed behavior. Only a final generic
// chain success reaches persisted-intent and domain verification.
func (v *ComposedVerifier) Verify(ctx context.Context, value execution.Execution, encoded string) (runtime.VerificationResult, error) {
	observation, err := v.provider.VerifyObservation(ctx, value, encoded)
	if err != nil {
		return runtime.VerificationResult{}, err
	}
	if !observation.FinalSuccess {
		return observation.Result, nil
	}

	scope, found := storage.ScopeFromContext(ctx)
	if !found {
		return runtime.VerificationResult{}, fmt.Errorf("persistence scope is unavailable for domain verification")
	}
	request := value.Request()
	intent, err := v.intents.FindIntentByID(ctx, scope, request.IntentID())
	if err != nil {
		return runtime.VerificationResult{}, fmt.Errorf("find frozen intent for domain verification: %w", err)
	}
	if intent.IntentID() != request.IntentID() || intent.Version() != request.IntentVersion() || intent.Digest() != request.IntentDigest() {
		return runtime.VerificationResult{}, fmt.Errorf("frozen intent does not match execution request")
	}
	if err := intent.Validate(); err != nil {
		return runtime.VerificationResult{}, fmt.Errorf("frozen intent is invalid: %w", err)
	}
	if intent.Status() != intents.StatusApproved && intent.Status() != intents.StatusReadyForExecution {
		return runtime.VerificationResult{}, fmt.Errorf("frozen intent is not execution-authorized")
	}

	switch intent.Type() {
	case intents.TypePayroll:
		plan, planErr := v.payroll.Plan(intent)
		if planErr != nil {
			return runtime.VerificationResult{}, fmt.Errorf("plan payroll intent for domain verification: %w", planErr)
		}
		domain, verifyErr := v.payrollV.Verify(intent, plan, observation.Receipt)
		if verifyErr != nil {
			return runtime.VerificationResult{}, fmt.Errorf("verify payroll domain: %w", verifyErr)
		}
		return mapPayrollResult(observation.Result, domain)
	case intents.TypeSwap:
		plan, planErr := v.swap.Plan(intent)
		if planErr != nil {
			return runtime.VerificationResult{}, fmt.Errorf("plan swap intent for domain verification: %w", planErr)
		}
		domain, verifyErr := v.swapV.Verify(intent, plan, observation.Receipt)
		if verifyErr != nil {
			return runtime.VerificationResult{}, fmt.Errorf("verify swap domain: %w", verifyErr)
		}
		return mapSwapResult(observation.Result, domain)
	default:
		return runtime.VerificationResult{}, fmt.Errorf("unsupported intent type %q for domain verification", intent.Type())
	}
}

func mapPayrollResult(generic runtime.VerificationResult, domain payroll.DomainResult) (runtime.VerificationResult, error) {
	switch domain.Status {
	case payroll.DomainVerified:
		return generic, nil
	case payroll.DomainAggregateOnly, payroll.DomainUnverified:
		return genericPending(generic), nil
	case payroll.DomainFailed:
		if !domain.DefinitiveFailure() {
			return runtime.VerificationResult{}, fmt.Errorf("payroll domain verification failed without definitive on-chain contradiction: %s", domain.ReasonCode)
		}
		return runtime.VerificationResult{Outcome: runtime.VerificationFailed, ReasonCode: domain.ReasonCode, ObservedAt: generic.ObservedAt}, nil
	default:
		return runtime.VerificationResult{}, fmt.Errorf("unknown payroll domain result %q", domain.Status)
	}
}

func mapSwapResult(generic runtime.VerificationResult, domain swap.DomainResult) (runtime.VerificationResult, error) {
	switch domain.Status {
	case swap.DomainVerified:
		return generic, nil
	case swap.DomainUnverified:
		return genericPending(generic), nil
	case swap.DomainFailed:
		if !domain.DefinitiveFailure() {
			return runtime.VerificationResult{}, fmt.Errorf("swap domain verification failed without definitive on-chain contradiction: %s", domain.ReasonCode)
		}
		return runtime.VerificationResult{Outcome: runtime.VerificationFailed, ReasonCode: domain.ReasonCode, ObservedAt: generic.ObservedAt}, nil
	default:
		return runtime.VerificationResult{}, fmt.Errorf("unknown swap domain result %q", domain.Status)
	}
}

func genericPending(generic runtime.VerificationResult) runtime.VerificationResult {
	return runtime.VerificationResult{Outcome: runtime.VerificationPending, Reference: generic.Reference, ObservedAt: generic.ObservedAt}
}
