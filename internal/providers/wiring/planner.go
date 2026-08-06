package wiring

import (
	"context"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/payroll"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/swap"
)

// Planner adapts the typed domain planners to the provider planner port.
//
// The execution request contains references only. The persisted intent is
// therefore the sole source of wallet identity, financial material, and the
// sealed contract call used to build the provider plan.
type Planner struct {
	intents storage.IntentRepository
	payroll payroll.Planner
	swap    swap.Planner
}

// NewPlanner assembles a provider planner from the frozen-intent repository
// and the typed domain planners. Domain planners remain separate so a payroll
// plan can never be produced by swap logic, or vice versa.
func NewPlanner(repository storage.IntentRepository, payrollPlanner payroll.Planner, swapPlanner swap.Planner) (*Planner, error) {
	if repository == nil {
		return nil, fmt.Errorf("intent repository is required")
	}
	return &Planner{intents: repository, payroll: payrollPlanner, swap: swapPlanner}, nil
}

var _ providers.Planner = (*Planner)(nil)

// Plan resolves and validates the exact frozen intent referenced by request,
// then converts the matching typed domain plan into a closed provider plan.
func (p *Planner) Plan(ctx context.Context, request execution.Request) (providers.Plan, error) {
	if p == nil || p.intents == nil {
		return providers.Plan{}, fmt.Errorf("intent repository is required")
	}
	if err := request.Validate(); err != nil {
		return providers.Plan{}, fmt.Errorf("execution request is invalid: %w", err)
	}
	scope, found := storage.ScopeFromContext(ctx)
	if !found {
		return providers.Plan{}, fmt.Errorf("persistence scope is unavailable for planner")
	}
	intent, err := p.intents.FindIntentByID(ctx, scope, request.IntentID())
	if err != nil {
		return providers.Plan{}, fmt.Errorf("find frozen intent: %w", err)
	}
	if intent.IntentID() != request.IntentID() || intent.Version() != request.IntentVersion() || intent.Digest() != request.IntentDigest() {
		return providers.Plan{}, fmt.Errorf("frozen intent does not match execution request")
	}
	if err := intent.Validate(); err != nil {
		return providers.Plan{}, fmt.Errorf("frozen intent is invalid: %w", err)
	}
	if intent.Status() != intents.StatusApproved {
		return providers.Plan{}, fmt.Errorf("frozen intent is not approved")
	}

	switch intent.Type() {
	case intents.TypePayroll:
		domainPlan, err := p.payroll.Plan(intent)
		if err != nil {
			return providers.Plan{}, fmt.Errorf("plan payroll intent: %w", err)
		}
		return contractPlan(intent, domainPlan.IntentID(), domainPlan.IntentDigest(), domainPlan.EncodedCall())
	case intents.TypeSwap:
		domainPlan, err := p.swap.Plan(intent)
		if err != nil {
			return providers.Plan{}, fmt.Errorf("plan swap intent: %w", err)
		}
		return contractPlan(intent, domainPlan.IntentID(), domainPlan.IntentDigest(), domainPlan.EncodedCall())
	default:
		return providers.Plan{}, fmt.Errorf("unsupported intent type %q", intent.Type())
	}
}

func contractPlan(intent intents.Intent, planIntentID, planDigest string, call contracts.EncodedCall) (providers.Plan, error) {
	if planIntentID != intent.IntentID() || planDigest != intent.Digest() {
		return providers.Plan{}, fmt.Errorf("domain plan does not match frozen intent")
	}
	owner := intent.Ownership()
	deadlines := []time.Time{intent.ExpiresAt(), intent.Constraints().Deadline}
	if financial := intent.Financial().Swap; financial != nil {
		deadlines = append(deadlines, financial.Deadline)
		if financial.Quote != nil {
			deadlines = append(deadlines, financial.Quote.ExpiresAt)
		}
	}
	plan, err := providers.NewContractExecutionPlan(providers.ContractExecutionParams{
		WalletBindingID: owner.WalletBindingID,
		WalletID:        owner.WalletID,
		WalletAddress:   owner.WalletAddress,
		ChainID:         owner.ChainID,
		Network:         owner.Network,
		Call:            call,
		SubmitNotAfter:  providers.EarliestDeadline(deadlines...),
	})
	if err != nil {
		return providers.Plan{}, fmt.Errorf("build contract execution plan: %w", err)
	}
	return plan, nil
}
