package wiring

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractpayroll "github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	contractswap "github.com/deseti/wizpay-mcp/internal/contracts/swap"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/execution/runtime"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/payroll"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/swap"
	"github.com/ethereum/go-ethereum/common"
)

type composedChainStub struct {
	receipt providers.Receipt
	err     error
	calls   int
}

func (s *composedChainStub) TransactionReceipt(context.Context, string, string) (providers.Receipt, error) {
	s.calls++
	return s.receipt, s.err
}

type composedResolverStub struct{}

func (composedResolverStub) ResolveReference(_ context.Context, _ string, reference providers.Reference) (providers.Reference, error) {
	return reference, nil
}

func newComposedVerifier(t *testing.T, intent intents.Intent, chain *composedChainStub) (*ComposedVerifier, *intentRepositoryStub, execution.Execution, context.Context) {
	t.Helper()
	request, approved := executionRequest(t, intent)
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := providers.NewVerifier(chain, composedResolverStub{}, providers.VerifierConfig{MinConfirmations: 1}, func() time.Time { return plannerTestNow })
	if err != nil {
		t.Fatal(err)
	}
	repository := &intentRepositoryStub{intent: approved}
	payrollPlanner := payroll.NewPlanner(nil)
	swapPlanner := swap.NewPlanner(nil)
	payrollVerifier := payroll.NewVerifier(nil)
	swapVerifier := swap.NewVerifier(nil)
	composed, err := NewComposedVerifier(provider, repository, &payrollPlanner, &swapPlanner, &payrollVerifier, &swapVerifier)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := storage.NewScope("tenant", "actor", "request", "trace")
	if err != nil {
		t.Fatal(err)
	}
	return composed, repository, value, storage.WithScope(context.Background(), scope)
}

func composedReference(t *testing.T) string {
	t.Helper()
	encoded, err := (providers.Reference{
		Provider: providers.ProviderCircleUserControlled, ChainID: contracts.ChainIDArcTestnet,
		WalletID: "wallet", ProviderTransactionID: "provider-tx", TransactionHash: composedTxHash,
	}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

const composedTxHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func composedSuccessReceipt(logs ...providers.ReceiptLog) providers.Receipt {
	return providers.Receipt{
		Status: providers.ReceiptSuccess, ChainID: contracts.ChainIDArcTestnet,
		TransactionHash: composedTxHash, BlockNumber: 10,
		BlockHash:     "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Confirmations: 1, Logs: logs,
	}
}

func TestNewComposedVerifierRejectsMissingMandatoryDependency(t *testing.T) {
	chain := &composedChainStub{}
	provider, err := providers.NewVerifier(chain, composedResolverStub{}, providers.VerifierConfig{MinConfirmations: 1}, func() time.Time { return plannerTestNow })
	if err != nil {
		t.Fatal(err)
	}
	repository := &intentRepositoryStub{}
	payrollPlanner := payroll.NewPlanner(nil)
	swapPlanner := swap.NewPlanner(nil)
	payrollVerifier := payroll.NewVerifier(nil)
	swapVerifier := swap.NewVerifier(nil)
	tests := []struct {
		name            string
		provider        *providers.Verifier
		repository      storage.IntentRepository
		payrollPlanner  *payroll.Planner
		swapPlanner     *swap.Planner
		payrollVerifier *payroll.Verifier
		swapVerifier    *swap.Verifier
	}{
		{name: "provider", repository: repository, payrollPlanner: &payrollPlanner, swapPlanner: &swapPlanner, payrollVerifier: &payrollVerifier, swapVerifier: &swapVerifier},
		{name: "intent repository", provider: provider, payrollPlanner: &payrollPlanner, swapPlanner: &swapPlanner, payrollVerifier: &payrollVerifier, swapVerifier: &swapVerifier},
		{name: "payroll planner", provider: provider, repository: repository, swapPlanner: &swapPlanner, payrollVerifier: &payrollVerifier, swapVerifier: &swapVerifier},
		{name: "swap planner", provider: provider, repository: repository, payrollPlanner: &payrollPlanner, payrollVerifier: &payrollVerifier, swapVerifier: &swapVerifier},
		{name: "payroll verifier", provider: provider, repository: repository, payrollPlanner: &payrollPlanner, swapPlanner: &swapPlanner, swapVerifier: &swapVerifier},
		{name: "swap verifier", provider: provider, repository: repository, payrollPlanner: &payrollPlanner, swapPlanner: &swapPlanner, payrollVerifier: &payrollVerifier},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if verifier, err := NewComposedVerifier(test.provider, test.repository, test.payrollPlanner, test.swapPlanner, test.payrollVerifier, test.swapVerifier); err == nil || verifier != nil {
				t.Fatalf("missing %s produced verifier %#v, err = %v", test.name, verifier, err)
			}
		})
	}
}

func TestComposedVerifierSwapDomainMapping(t *testing.T) {
	intent := frozenIntent(t, intents.TypeSwap)
	financial := intent.Financial().Swap
	validLog := swapDomainLog(t, intent.Ownership().WalletAddress, financial.Router, financial.InputToken.Address, financial.OutputToken.Address, big.NewInt(10000000), big.NewInt(1000000), big.NewInt(9000000), big.NewInt(9000000), financial.Recipient)
	cases := []struct {
		name       string
		logs       []providers.ReceiptLog
		want       runtime.VerificationOutcome
		wantReason string
	}{
		{name: "verified", logs: []providers.ReceiptLog{validLog}, want: runtime.VerificationVerified},
		{name: "unverified", want: runtime.VerificationPending},
		{name: "contradictory", logs: []providers.ReceiptLog{swapDomainLog(t, intent.Ownership().WalletAddress, financial.Router, financial.InputToken.Address, financial.OutputToken.Address, big.NewInt(10000000), big.NewInt(1000000), big.NewInt(9000000), big.NewInt(1000000), financial.Recipient)}, want: runtime.VerificationFailed, wantReason: "SWAP_EXECUTED_AMOUNT_OUT_BELOW_MINIMUM"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			chain := &composedChainStub{receipt: composedSuccessReceipt(test.logs...)}
			verifier, _, value, ctx := newComposedVerifier(t, frozenIntent(t, intents.TypeSwap), chain)
			result, err := verifier.Verify(ctx, value, composedReference(t))
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != test.want || result.ReasonCode != test.wantReason {
				t.Fatalf("result = %#v, want outcome %s reason %q", result, test.want, test.wantReason)
			}
			if chain.calls != 1 {
				t.Fatalf("receipt calls = %d, want 1", chain.calls)
			}
		})
	}
}

func TestComposedVerifierPayrollDomainMapping(t *testing.T) {
	intent := frozenIntent(t, intents.TypePayroll)
	financial := intent.Financial().Payroll
	line := financial.Recipients[0]
	cases := []struct {
		name       string
		logs       []providers.ReceiptLog
		want       runtime.VerificationOutcome
		wantReason string
	}{
		{name: "single verified", logs: []providers.ReceiptLog{payrollDomainLog(t, intent.Ownership().WalletAddress, line.Address, financial.TokenIn.Address, line.TokenOut.Address, big.NewInt(1000000), big.NewInt(1000000), big.NewInt(0))}, want: runtime.VerificationVerified},
		{name: "definitive contradiction", logs: []providers.ReceiptLog{payrollDomainLog(t, intent.Ownership().WalletAddress, line.Address, financial.TokenIn.Address, line.TokenOut.Address, big.NewInt(1000000), big.NewInt(0), big.NewInt(0))}, want: runtime.VerificationFailed, wantReason: "PAYMENT_ROUTED_AMOUNT_OUT_BELOW_MINIMUM"},
		{name: "missing event", want: runtime.VerificationPending},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			chain := &composedChainStub{receipt: composedSuccessReceipt(test.logs...)}
			verifier, _, value, ctx := newComposedVerifier(t, frozenIntent(t, intents.TypePayroll), chain)
			result, err := verifier.Verify(ctx, value, composedReference(t))
			if err != nil || result.Outcome != test.want || result.ReasonCode != test.wantReason {
				t.Fatalf("result = %#v, err = %v", result, err)
			}
		})
	}
}

func TestComposedVerifierAcceptsReadyForExecutionPayrollIntent(t *testing.T) {
	intent := frozenIntent(t, intents.TypePayroll)
	financial := intent.Financial().Payroll
	line := financial.Recipients[0]
	log := payrollDomainLog(t, intent.Ownership().WalletAddress, line.Address, financial.TokenIn.Address, line.TokenOut.Address, big.NewInt(1000000), big.NewInt(1000000), big.NewInt(0))
	assertReadyForExecutionDomainVerified(t, intent, composedSuccessReceipt(log))
}

func TestComposedVerifierAcceptsReadyForExecutionSwapIntent(t *testing.T) {
	intent := frozenIntent(t, intents.TypeSwap)
	financial := intent.Financial().Swap
	log := swapDomainLog(t, intent.Ownership().WalletAddress, financial.Router, financial.InputToken.Address, financial.OutputToken.Address, big.NewInt(10000000), big.NewInt(1000000), big.NewInt(9000000), big.NewInt(9000000), financial.Recipient)
	assertReadyForExecutionDomainVerified(t, intent, composedSuccessReceipt(log))
}

func assertReadyForExecutionDomainVerified(t *testing.T, intent intents.Intent, receipt providers.Receipt) {
	t.Helper()
	request, approved := executionRequest(t, intent)
	ready, err := approved.Transition(intents.StatusReadyForExecution, plannerTestNow.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	chain := &composedChainStub{receipt: receipt}
	provider, err := providers.NewVerifier(chain, composedResolverStub{}, providers.VerifierConfig{MinConfirmations: 1}, func() time.Time { return plannerTestNow })
	if err != nil {
		t.Fatal(err)
	}
	payrollPlanner := payroll.NewPlanner(nil)
	swapPlanner := swap.NewPlanner(nil)
	payrollVerifier := payroll.NewVerifier(nil)
	swapVerifier := swap.NewVerifier(nil)
	verifier, err := NewComposedVerifier(provider, &intentRepositoryStub{intent: ready}, &payrollPlanner, &swapPlanner, &payrollVerifier, &swapVerifier)
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := storage.NewScope("tenant", "actor", "request", "trace")
	result, err := verifier.Verify(storage.WithScope(context.Background(), scope), value, composedReference(t))
	if err != nil || result.Outcome != runtime.VerificationVerified {
		t.Fatalf("READY_FOR_EXECUTION result = %#v, err = %v", result, err)
	}
	if chain.calls != 1 {
		t.Fatalf("receipt calls = %d, want 1", chain.calls)
	}
}

func TestComposedVerifierPayrollBatchIsPending(t *testing.T) {
	intent := frozenBatchIntent(t)
	financial := intent.Financial().Payroll
	line := financial.Recipients[0]
	chain := &composedChainStub{receipt: composedSuccessReceipt(batchDomainLog(t, intent.Ownership().WalletAddress, financial.TokenIn.Address, line.TokenOut.Address, big.NewInt(1000000), big.NewInt(1000000), big.NewInt(0), big.NewInt(1), financial.ReferenceID))}
	verifier, _, value, ctx := newComposedVerifier(t, intent, chain)
	result, err := verifier.Verify(ctx, value, composedReference(t))
	if err != nil || result.Outcome != runtime.VerificationPending || result.Reference == "" {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

func TestComposedVerifierRejectsPersistedBindingAndScope(t *testing.T) {
	request, approved := executionRequest(t, frozenIntent(t, intents.TypePayroll))
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	chain := &composedChainStub{receipt: composedSuccessReceipt()}
	provider, err := providers.NewVerifier(chain, composedResolverStub{}, providers.VerifierConfig{MinConfirmations: 1}, func() time.Time { return plannerTestNow })
	if err != nil {
		t.Fatal(err)
	}
	baseScope, _ := storage.NewScope("tenant", "actor", "request", "trace")
	cases := []struct {
		name   string
		intent intents.Intent
		ctx    context.Context
	}{
		{name: "id mismatch", intent: mustApproved(t, frozenIntentVariant(t, intents.TypeSwap, "same")), ctx: storage.WithScope(context.Background(), baseScope)},
		{name: "version mismatch", intent: mustApproved(t, frozenIntentVersion(t, intents.TypePayroll, "nonce", 2)), ctx: storage.WithScope(context.Background(), baseScope)},
		{name: "digest mismatch", intent: mustApproved(t, frozenIntentVariant(t, intents.TypePayroll, "different")), ctx: storage.WithScope(context.Background(), baseScope)},
		{name: "approval required", intent: frozenIntent(t, intents.TypePayroll), ctx: storage.WithScope(context.Background(), baseScope)},
		{name: "cancelled", intent: intentAtStatus(t, approved, intents.StatusCancelled), ctx: storage.WithScope(context.Background(), baseScope)},
		{name: "expired", intent: intentAtStatus(t, approved, intents.StatusExpired), ctx: storage.WithScope(context.Background(), baseScope)},
		{name: "missing scope", intent: approved, ctx: context.Background()},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			repository := &intentRepositoryStub{intent: test.intent}
			payrollPlanner := payroll.NewPlanner(nil)
			swapPlanner := swap.NewPlanner(nil)
			payrollVerifier := payroll.NewVerifier(nil)
			swapVerifier := swap.NewVerifier(nil)
			verifier, err := NewComposedVerifier(provider, repository, &payrollPlanner, &swapPlanner, &payrollVerifier, &swapVerifier)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := verifier.Verify(test.ctx, value, composedReference(t)); err == nil {
				t.Fatal("expected persisted binding/scope rejection")
			}
		})
	}
}

func TestComposedVerifierReorgDoesNotRunDomainGate(t *testing.T) {
	intent := frozenIntent(t, intents.TypeSwap)
	financial := intent.Financial().Swap
	log := swapDomainLog(t, intent.Ownership().WalletAddress, financial.Router, financial.InputToken.Address, financial.OutputToken.Address, big.NewInt(10000000), big.NewInt(1000000), big.NewInt(9000000), big.NewInt(9000000), financial.Recipient)
	chain := &composedChainStub{receipt: composedSuccessReceipt(log)}
	verifier, repository, value, ctx := newComposedVerifier(t, intent, chain)
	first, err := verifier.Verify(ctx, value, composedReference(t))
	if err != nil || first.Outcome != runtime.VerificationVerified {
		t.Fatalf("first result = %#v, err = %v", first, err)
	}
	chain.receipt.BlockHash = "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	second, err := verifier.Verify(ctx, value, first.Reference)
	if err != nil || second.Outcome != runtime.VerificationPending {
		t.Fatalf("reorg result = %#v, err = %v", second, err)
	}
	if repository.findCalls != 1 || chain.calls != 2 {
		t.Fatalf("domain/reconciliation calls = %d/%d, want 1/2", repository.findCalls, chain.calls)
	}
}

func TestComposedVerifierDeterministicSameObservation(t *testing.T) {
	intent := frozenIntent(t, intents.TypeSwap)
	financial := intent.Financial().Swap
	log := swapDomainLog(t, intent.Ownership().WalletAddress, financial.Router, financial.InputToken.Address, financial.OutputToken.Address, big.NewInt(10000000), big.NewInt(1000000), big.NewInt(9000000), big.NewInt(9000000), financial.Recipient)
	chain := &composedChainStub{receipt: composedSuccessReceipt(log)}
	verifier, _, value, ctx := newComposedVerifier(t, intent, chain)
	reference := composedReference(t)
	first, err := verifier.Verify(ctx, value, reference)
	if err != nil {
		t.Fatal(err)
	}
	second, err := verifier.Verify(ctx, value, first.Reference)
	if err != nil || first.Outcome != second.Outcome || first.Reference != second.Reference || first.ReasonCode != second.ReasonCode {
		t.Fatalf("non-deterministic results: first=%#v second=%#v err=%v", first, second, err)
	}
}

func TestComposedVerifierNonDefinitiveDomainFailuresRecover(t *testing.T) {
	generic := runtime.VerificationResult{Outcome: runtime.VerificationVerified, Reference: "reference", ObservedAt: plannerTestNow}
	if result, err := mapPayrollResult(generic, payroll.DomainResult{Status: payroll.DomainFailed, ReasonCode: "PAYROLL_PLAN_MISMATCH"}); err == nil || result.Outcome == runtime.VerificationFailed {
		t.Fatalf("payroll non-definitive failure mapped terminally: result=%#v err=%v", result, err)
	}
	if result, err := mapSwapResult(generic, swap.DomainResult{Status: swap.DomainFailed, ReasonCode: "SWAP_PLAN_MISMATCH"}); err == nil || result.Outcome == runtime.VerificationFailed {
		t.Fatalf("swap non-definitive failure mapped terminally: result=%#v err=%v", result, err)
	}
}

func intentAtStatus(t *testing.T, intent intents.Intent, status intents.Status) intents.Intent {
	t.Helper()
	if intent.Status() == status {
		return intent
	}
	at := plannerTestNow.Add(4 * time.Second)
	if status == intents.StatusExpired {
		at = intent.ExpiresAt()
	}
	transitioned, err := intent.Transition(status, at)
	if err != nil {
		t.Fatal(err)
	}
	return transitioned
}

func mustApproved(t *testing.T, intent intents.Intent) intents.Intent {
	t.Helper()
	approved, err := approveIntent(t, intent)
	if err != nil {
		t.Fatal(err)
	}
	return approved
}

func payrollDomainLog(t *testing.T, sender, recipient, tokenIn, tokenOut string, amountIn, amountOut, fee *big.Int) providers.ReceiptLog {
	t.Helper()
	event, err := contractpayroll.EventBySignature(contractpayroll.SigPaymentRouted)
	if err != nil {
		t.Fatal(err)
	}
	data, err := event.Inputs.NonIndexed().Pack(common.HexToAddress(tokenIn), common.HexToAddress(tokenOut), amountIn, amountOut, fee)
	if err != nil {
		t.Fatal(err)
	}
	return providers.ReceiptLog{Address: contracts.AddressWizPayPayroll, Topics: [][]byte{event.ID.Bytes(), contracts.TopicAddress(sender), contracts.TopicAddress(recipient)}, Data: data}
}

func batchDomainLog(t *testing.T, sender, tokenIn, tokenOut string, totalIn, totalOut, fees, count *big.Int, reference string) providers.ReceiptLog {
	t.Helper()
	event, err := contractpayroll.EventBySignature(contractpayroll.SigBatchPaymentRouted)
	if err != nil {
		t.Fatal(err)
	}
	data, err := event.Inputs.NonIndexed().Pack(common.HexToAddress(tokenIn), common.HexToAddress(tokenOut), totalIn, totalOut, fees, count, reference)
	if err != nil {
		t.Fatal(err)
	}
	return providers.ReceiptLog{Address: contracts.AddressWizPayPayroll, Topics: [][]byte{event.ID.Bytes(), contracts.TopicAddress(sender)}, Data: data}
}

func swapDomainLog(t *testing.T, user, router, tokenIn, tokenOut string, amountIn, fee, netIn, amountOut *big.Int, recipient string) providers.ReceiptLog {
	t.Helper()
	event, err := contractswap.EventBySignature(contractswap.SigWizPaySwapExecuted)
	if err != nil {
		t.Fatal(err)
	}
	data, err := event.Inputs.NonIndexed().Pack(common.HexToAddress(tokenOut), amountIn, fee, netIn, amountOut, common.HexToAddress(recipient))
	if err != nil {
		t.Fatal(err)
	}
	return providers.ReceiptLog{Address: contracts.AddressWizPaySwapExecutor, Topics: [][]byte{event.ID.Bytes(), contracts.TopicAddress(user), contracts.TopicAddress(router), contracts.TopicAddress(tokenIn)}, Data: data}
}

func frozenBatchIntent(t *testing.T) intents.Intent {
	t.Helper()
	owner := intents.Ownership{UserID: "user", IdentityProvider: "circle", ProviderUserReference: "provider-user", WalletBindingID: "binding", WalletBindingVersion: 1, WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcTestnet, Network: "TESTNET"}
	tokenIn := intents.Token{ChainID: contracts.ChainIDArcTestnet, Standard: "ERC20", Address: "0x1111111111111111111111111111111111111111", Symbol: "USDC", Decimals: 6}
	tokenOut := intents.Token{ChainID: contracts.ChainIDArcTestnet, Standard: "ERC20", Address: "0x5555555555555555555555555555555555555555", Symbol: "EURC", Decimals: 6}
	params := intents.Params{IntentID: "intent-payroll-batch", Version: 1, ClientRequestID: "client-batch", Nonce: "nonce-batch", Type: intents.TypePayroll, Ownership: owner, Route: intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferencePayroll, Version: 1}, Constraints: intents.Constraints{Deadline: plannerTestNow.Add(20 * time.Minute), PolicyReference: "policy:1"}, CreatedAt: plannerTestNow, ExpiresAt: plannerTestNow.Add(30 * time.Minute), Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{SchemaVersion: intents.FinancialSchemaPhase12, Variant: intents.PayrollVariantBatchSingleTokenOut, TokenIn: tokenIn, Recipients: []intents.Recipient{{Address: "0x3333333333333333333333333333333333333333", TokenOut: tokenOut, AmountIn: amount("1"), MinAmountOut: amount("1")}}, Total: amount("1"), ReferenceID: "batch"}}}
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
