package wiring

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/payroll"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/swap"
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

var plannerTestNow = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

type intentRepositoryStub struct {
	intent         intents.Intent
	err            error
	requestedID    string
	requestedScope storage.Scope
	findCalls      int
}

func (s *intentRepositoryStub) FindIntentByID(_ context.Context, scope storage.Scope, id string) (intents.Intent, error) {
	s.requestedID = id
	s.requestedScope = scope
	s.findCalls++
	return s.intent, s.err
}
func (intentRepositoryStub) FindIntentByClientRequestID(context.Context, storage.Scope, string) (intents.Intent, error) {
	panic("not used")
}
func (intentRepositoryStub) FindIntentByOperationKey(context.Context, storage.Scope, string, uint64) (intents.Intent, error) {
	panic("not used")
}
func (intentRepositoryStub) CreateIntent(context.Context, storage.Scope, intents.Intent) (storage.CreateIntentResult, error) {
	panic("not used")
}
func (intentRepositoryStub) FreezeIntent(context.Context, storage.Scope, intents.Intent, uint64) (intents.Intent, error) {
	panic("not used")
}
func (intentRepositoryStub) UpdateIntent(context.Context, storage.Scope, intents.Intent, uint64) (intents.Intent, error) {
	panic("not used")
}

func TestPlannerBuildsPayrollAndSwapContractPlans(t *testing.T) {
	for _, kind := range []intents.Type{intents.TypePayroll, intents.TypeSwap} {
		t.Run(string(kind), func(t *testing.T) {
			request, intent := executionRequest(t, frozenIntent(t, kind))
			scope, err := storage.NewScope("tenant", "actor", "request", "trace")
			if err != nil {
				t.Fatal(err)
			}
			repository := &intentRepositoryStub{intent: intent}
			planner, err := NewPlanner(repository, payroll.NewPlanner(nil), swap.NewPlanner(nil))
			if err != nil {
				t.Fatal(err)
			}
			plan, err := planner.Plan(storage.WithScope(context.Background(), scope), request)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			if plan.EffectiveKind() != providers.PlanKindContractExecution {
				t.Fatalf("kind = %s", plan.EffectiveKind())
			}
			call, ok := plan.EncodedCall()
			if !ok {
				t.Fatal("expected sealed contract call")
			}
			wantContract, wantTarget := contracts.ContractWizPayPayroll, contracts.AddressWizPayPayroll
			wantDeadline := intent.ExpiresAt()
			if kind == intents.TypeSwap {
				wantContract, wantTarget = contracts.ContractWizPaySwapExecutor, contracts.AddressWizPaySwapExecutor
				financial := intent.Financial().Swap
				wantDeadline = providers.EarliestDeadline(wantDeadline, intent.Constraints().Deadline, financial.Deadline, financial.Quote.ExpiresAt)
			} else {
				wantDeadline = providers.EarliestDeadline(wantDeadline, intent.Constraints().Deadline)
			}
			if call.ContractID() != wantContract || call.To() != wantTarget {
				t.Fatalf("call binding = %q/%s, want %q/%s", call.ContractID(), call.To(), wantContract, wantTarget)
			}
			gotDeadline, ok := plan.SubmitNotAfter()
			if !ok || !gotDeadline.Equal(wantDeadline) {
				t.Fatalf("SubmitNotAfter = %v/%t, want %v", gotDeadline, ok, wantDeadline)
			}
			if repository.findCalls != 1 || repository.requestedID != request.IntentID() || repository.requestedScope.TenantID() != "tenant" || repository.requestedScope.ActorID() != "actor" {
				t.Fatalf("repository lookup = calls %d, id %q, scope tenant=%q actor=%q", repository.findCalls, repository.requestedID, repository.requestedScope.TenantID(), repository.requestedScope.ActorID())
			}
		})
	}
}

func TestPlannerSameRequestProducesIdenticalContractPlan(t *testing.T) {
	request, intent := executionRequest(t, frozenIntent(t, intents.TypeSwap))
	repository := &intentRepositoryStub{intent: intent}
	planner, err := NewPlanner(repository, payroll.NewPlanner(nil), swap.NewPlanner(nil))
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := storage.NewScope("tenant", "actor", "request", "trace")
	first, err := planner.Plan(storage.WithScope(context.Background(), scope), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := planner.Plan(storage.WithScope(context.Background(), scope), request)
	if err != nil {
		t.Fatal(err)
	}
	firstCall, _ := first.EncodedCall()
	secondCall, _ := second.EncodedCall()
	firstDeadline, _ := first.SubmitNotAfter()
	secondDeadline, _ := second.SubmitNotAfter()
	if first.EffectiveKind() != second.EffectiveKind() || firstCall.ContractID() != secondCall.ContractID() || firstCall.To() != secondCall.To() || !bytes.Equal(firstCall.CallData(), secondCall.CallData()) || !firstDeadline.Equal(secondDeadline) {
		t.Fatal("same request produced different provider contract plans")
	}
}

func TestPlannerRejectsPersistedNonApprovedIntent(t *testing.T) {
	request, approved := executionRequest(t, frozenIntent(t, intents.TypePayroll))
	repository := &intentRepositoryStub{intent: frozenIntent(t, intents.TypePayroll)}
	if repository.intent.IntentID() != approved.IntentID() {
		t.Fatal("test fixture intent IDs differ")
	}
	planner, _ := NewPlanner(repository, payroll.NewPlanner(nil), swap.NewPlanner(nil))
	scope, _ := storage.NewScope("tenant", "actor", "request", "trace")
	if _, err := planner.Plan(storage.WithScope(context.Background(), scope), request); err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("expected persisted authorization rejection, got %v", err)
	}
}

func TestPlannerRejectsIntentVersionMismatch(t *testing.T) {
	request, _ := executionRequest(t, frozenIntent(t, intents.TypePayroll))
	persisted, err := approveIntent(t, frozenIntentVersion(t, intents.TypePayroll, "nonce", 2))
	if err != nil {
		t.Fatal(err)
	}
	repository := &intentRepositoryStub{intent: persisted}
	planner, _ := NewPlanner(repository, payroll.NewPlanner(nil), swap.NewPlanner(nil))
	scope, _ := storage.NewScope("tenant", "actor", "request", "trace")
	if _, err := planner.Plan(storage.WithScope(context.Background(), scope), request); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected version mismatch rejection, got %v", err)
	}
}

func TestPlannerRejectsDigestMismatch(t *testing.T) {
	request, _ := executionRequest(t, frozenIntentVariant(t, intents.TypePayroll, "different-nonce"))
	intent, err := approveIntent(t, frozenIntent(t, intents.TypePayroll))
	planner, err := NewPlanner(&intentRepositoryStub{intent: intent}, payroll.NewPlanner(nil), swap.NewPlanner(nil))
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := storage.NewScope("tenant", "actor", "request", "trace")
	if _, err := planner.Plan(storage.WithScope(context.Background(), scope), request); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected digest mismatch rejection, got %v", err)
	}
}

func TestPlannerRejectsUnsupportedIntentType(t *testing.T) {
	request, intent := executionRequest(t, unsupportedFrozenIntent(t))
	scope, _ := storage.NewScope("tenant", "actor", "request", "trace")
	planner, err := NewPlanner(&intentRepositoryStub{intent: intent}, payroll.NewPlanner(nil), swap.NewPlanner(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planner.Plan(storage.WithScope(context.Background(), scope), request); err == nil || !strings.Contains(err.Error(), "unsupported intent type") {
		t.Fatalf("expected unsupported intent error, got %v", err)
	}
}

func TestPlannerPropagatesRepositoryFailure(t *testing.T) {
	request, _ := executionRequest(t, frozenIntent(t, intents.TypePayroll))
	scope, _ := storage.NewScope("tenant", "actor", "request", "trace")
	want := errors.New("repository unavailable")
	planner, err := NewPlanner(&intentRepositoryStub{err: want}, payroll.NewPlanner(nil), swap.NewPlanner(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planner.Plan(storage.WithScope(context.Background(), scope), request); !errors.Is(err, want) {
		t.Fatalf("error = %v, want repository error", err)
	}
}

func TestPlannerRejectsMissingScope(t *testing.T) {
	request, intent := executionRequest(t, frozenIntent(t, intents.TypePayroll))
	planner, _ := NewPlanner(&intentRepositoryStub{intent: intent}, payroll.NewPlanner(nil), swap.NewPlanner(nil))
	if _, err := planner.Plan(context.Background(), request); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("expected missing scope rejection, got %v", err)
	}
}

// The fixture follows the public authorization path so the adapter receives a
// real execution.Request rather than a synthetic request with inaccessible
// identity fields.
func executionRequest(t *testing.T, intent intents.Intent) (execution.Request, intents.Intent) {
	t.Helper()
	policy, err := policies.NewDraft(policies.Params{PolicyID: "policy", Version: 1, Name: "planner", Scope: policies.Scope{UserID: "user", WalletBindingID: "binding", IntentTypes: []intents.Type{intent.Type()}}, Rules: []policies.Rule{{RuleID: "allow", OnViolation: policies.DecisionDeny, OperationAllowlist: &policies.OperationAllowlistRule{Allowed: []intents.Type{intent.Type()}}}}, CreatedAt: plannerTestNow.Add(-time.Hour), ValidFrom: plannerTestNow.Add(-time.Minute), ExpiresAt: plannerTestNow.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	policy, err = policy.Transition(policies.StatusActive, plannerTestNow.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	identity, err := auth.NewIdentity("user", "circle", auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	identityContext, err := auth.NewIdentityContext(identity, auth.RequestMetadata{RequestID: "request"})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := wallet.NewBinding(wallet.BindingParams{BindingID: "binding", Version: 1, UserID: "user", Provider: "circle", ProviderUserReference: "provider-user", WalletID: "wallet", Address: intent.Ownership().WalletAddress, ChainID: intent.Ownership().ChainID, Network: intent.Ownership().Network, Status: wallet.BindingStatusActive, VerificationReference: "verification", CreatedAt: plannerTestNow.Add(-time.Hour), VerifiedAt: plannerTestNow.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	approval, err := approvals.New(approvals.Params{ApprovalID: "approval", Version: 1, ApprovalRequestID: "approval-request", CreatedAt: plannerTestNow.Add(time.Second), ExpiresAt: plannerTestNow.Add(30 * time.Minute)}, intent)
	if err != nil {
		t.Fatal(err)
	}
	approval, err = approval.Approve(plannerTestNow.Add(2 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Approve(approval, plannerTestNow.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	operation, err := intents.NewOperationIdentity(intent)
	if err != nil {
		t.Fatal(err)
	}
	result, err := policies.Evaluate(policy, intent, identityContext, binding, plannerTestNow.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	approval, err = approval.Consume(plannerTestNow.Add(6*time.Second), operation)
	if err != nil {
		t.Fatal(err)
	}
	request, err := execution.NewRequest(intent, approval, result, plannerTestNow.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return request, intent
}

func frozenIntent(t *testing.T, kind intents.Type) intents.Intent {
	return frozenIntentVariant(t, kind, "nonce")
}

func frozenIntentVariant(t *testing.T, kind intents.Type, nonce string) intents.Intent {
	return frozenIntentVersion(t, kind, nonce, 1)
}

func frozenIntentVersion(t *testing.T, kind intents.Type, nonce string, version uint64) intents.Intent {
	t.Helper()
	owner := intents.Ownership{UserID: "user", IdentityProvider: "circle", ProviderUserReference: "provider-user", WalletBindingID: "binding", WalletBindingVersion: 1, WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: "5042002", Network: "TESTNET"}
	token := intents.Token{ChainID: "5042002", Standard: "ERC20", Address: "0x1111111111111111111111111111111111111111", Symbol: "USDC", Decimals: 6}
	params := intents.Params{IntentID: "intent-" + strings.ToLower(string(kind)), Version: version, ClientRequestID: "client", Nonce: nonce, Type: kind, Ownership: owner, Route: intents.Route{Type: intents.RouteAllowlistedContract, Reference: map[intents.Type]string{intents.TypePayroll: intents.RouteReferencePayroll, intents.TypeSwap: intents.RouteReferenceSwap}[kind], Version: 1}, Constraints: intents.Constraints{Deadline: plannerTestNow.Add(20 * time.Minute), PolicyReference: "policy:1"}, CreatedAt: plannerTestNow, ExpiresAt: plannerTestNow.Add(30 * time.Minute)}
	if kind == intents.TypePayroll {
		params.Financial = intents.FinancialParameters{Payroll: &intents.PayrollParameters{SchemaVersion: intents.FinancialSchemaPhase12, Variant: intents.PayrollVariantSingle, TokenIn: token, Recipients: []intents.Recipient{{Address: "0x3333333333333333333333333333333333333333", TokenOut: token, AmountIn: amount("1"), MinAmountOut: amount("1")}}, Total: amount("1")}}
	} else {
		output := token
		output.Address = "0x5555555555555555555555555555555555555555"
		output.Symbol = "EURC"
		params.Financial = intents.FinancialParameters{Swap: &intents.SwapParameters{SchemaVersion: intents.FinancialSchemaPhase12, InputToken: token, OutputToken: output, InputAmount: amount("10"), ExpectedOutput: amount("9"), MinimumOutput: amount("8.91"), MaxSlippageBPS: 100, Router: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Recipient: owner.WalletAddress, Quote: &intents.SwapQuote{QuoteID: "quote", Source: "test", ExpectedAmountOut: amount("9"), MinAmountOut: amount("8.91"), Router: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpiresAt: plannerTestNow.Add(15 * time.Minute), EvidenceReference: "evidence"}, Deadline: plannerTestNow.Add(10 * time.Minute)}}
	}
	intent, err := intents.NewDraft(params)
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, plannerTestNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusApprovalRequired, plannerTestNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func approveIntent(t *testing.T, intent intents.Intent) (intents.Intent, error) {
	t.Helper()
	approval, err := approvals.New(approvals.Params{ApprovalID: "approval", Version: 1, ApprovalRequestID: "approval-request", CreatedAt: plannerTestNow.Add(time.Second), ExpiresAt: plannerTestNow.Add(30 * time.Minute)}, intent)
	if err != nil {
		return intents.Intent{}, err
	}
	approval, err = approval.Approve(plannerTestNow.Add(2 * time.Second))
	if err != nil {
		return intents.Intent{}, err
	}
	return intent.Approve(approval, plannerTestNow.Add(3*time.Second))
}

func amount(decimal string) intents.Amount {
	return intents.Amount{Decimal: decimal, BaseUnits: map[string]string{"1": "1000000", "8.91": "8910000", "9": "9000000", "10": "10000000"}[decimal], Decimals: 6}
}

func unsupportedFrozenIntent(t *testing.T) intents.Intent {
	// A valid legacy BRIDGE intent is enough to exercise routing; it is never
	// passed to a domain planner.
	params := intents.Params{IntentID: "intent-bridge", Version: 1, ClientRequestID: "client-bridge", Nonce: "nonce-bridge", Type: intents.TypeBridge, Ownership: intents.Ownership{UserID: "user", IdentityProvider: "circle", ProviderUserReference: "provider-user", WalletBindingID: "binding", WalletBindingVersion: 1, WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: "5042002", Network: "TESTNET"}, Financial: intents.FinancialParameters{Bridge: &intents.BridgeParameters{SourceChainID: "5042002", DestinationChainID: "1", SourceToken: intents.Token{ChainID: "5042002", Standard: "ERC20", Address: "0x1111111111111111111111111111111111111111", Symbol: "USDC", Decimals: 6}, SourceAmount: amount("1"), DestinationAmount: amount("1"), DestinationAddress: "0x3333333333333333333333333333333333333333", PlanReference: "bridge"}}, Route: intents.Route{Type: intents.RouteApprovedProvider, Reference: "bridge", Version: 1}, Constraints: intents.Constraints{Deadline: plannerTestNow.Add(20 * time.Minute), PolicyReference: "policy:1"}, CreatedAt: plannerTestNow, ExpiresAt: plannerTestNow.Add(30 * time.Minute)}
	intent, err := intents.NewDraft(params)
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, plannerTestNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusApprovalRequired, plannerTestNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return intent
}
