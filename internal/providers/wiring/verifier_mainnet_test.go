package wiring

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/execution/runtime"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/payroll"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/swap"
)

const mainnetVerifierHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type mainnetChainStub struct {
	receipt providers.Receipt
	calls   int
}

func (s *mainnetChainStub) TransactionReceipt(context.Context, string, string) (providers.Receipt, error) {
	s.calls++
	return s.receipt, nil
}

type mainnetResolverStub struct{}

func (mainnetResolverStub) ResolveReference(_ context.Context, _ string, reference providers.Reference) (providers.Reference, error) {
	return reference, nil
}

func mainnetReference(t *testing.T) string {
	t.Helper()
	encoded, err := (providers.Reference{Provider: providers.ProviderCircleUserControlled, ChainID: contracts.ChainIDArcMainnet, WalletID: "wallet", TransactionHash: mainnetVerifierHash}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func mainnetComposedVerifier(t *testing.T, kind intents.Type, receipt providers.Receipt) (*ComposedVerifier, *intentRepositoryStub, execution.Execution, context.Context, *mainnetChainStub) {
	t.Helper()
	request, approved := executionRequest(t, frozenIntent(t, kind))
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	chain := &mainnetChainStub{receipt: receipt}
	provider, err := providers.NewVerifier(chain, mainnetResolverStub{}, providers.VerifierConfig{MinConfirmations: 1}, fixedClock())
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
	scope, _ := storage.NewScope("tenant", "actor", "request", "trace")
	return composed, repository, value, storage.WithScope(context.Background(), scope), chain
}

func TestTrackAMainnetDomainResultsRemainPending(t *testing.T) {
	generic := runtime.VerificationResult{Outcome: runtime.VerificationVerified, Reference: "tx", ObservedAt: time.Now().UTC()}
	payrollResult, err := mapPayrollResult(generic, payroll.DomainResult{Status: payroll.DomainUnverified, ReasonCode: "ARC_MAINNET_PAYROLL_EXECUTION_DISABLED"})
	if err != nil || payrollResult.Outcome != runtime.VerificationPending {
		t.Fatalf("payroll result=%#v err=%v", payrollResult, err)
	}
	swapResult, err := mapSwapResult(generic, swap.DomainResult{Status: swap.DomainUnverified, ReasonCode: "ARC_MAINNET_SWAP_EXECUTION_DISABLED"})
	if err != nil || swapResult.Outcome != runtime.VerificationPending {
		t.Fatalf("swap result=%#v err=%v", swapResult, err)
	}
}

func TestTrackAComposedVerifierPreservesGenericPendingWithoutDomainLookup(t *testing.T) {
	receipt := providers.Receipt{Status: providers.ReceiptUnknown, ChainID: contracts.ChainIDArcMainnet, TransactionHash: mainnetVerifierHash}
	verifier, repository, value, ctx, chain := mainnetComposedVerifier(t, intents.TypeSwap, receipt)
	result, err := verifier.Verify(ctx, value, mainnetReference(t))
	if err != nil || result.Outcome != runtime.VerificationPending {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if repository.findCalls != 0 || chain.calls != 1 {
		t.Fatalf("repository/chain calls=%d/%d", repository.findCalls, chain.calls)
	}
}

func TestTrackAComposedVerifierCannotPromoteGenericSuccess(t *testing.T) {
	for _, kind := range []intents.Type{intents.TypePayroll, intents.TypeSwap} {
		t.Run(string(kind), func(t *testing.T) {
			receipt := providers.Receipt{Status: providers.ReceiptSuccess, ChainID: contracts.ChainIDArcMainnet, TransactionHash: mainnetVerifierHash, BlockNumber: 10, BlockHash: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Confirmations: 1}
			verifier, repository, value, ctx, chain := mainnetComposedVerifier(t, kind, receipt)
			result, err := verifier.Verify(ctx, value, mainnetReference(t))
			if err == nil || !strings.Contains(err.Error(), "disabled in Track A") || result.Outcome == runtime.VerificationVerified {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if repository.findCalls != 1 || chain.calls != 1 {
				t.Fatalf("repository/chain calls=%d/%d", repository.findCalls, chain.calls)
			}
		})
	}
}

func TestTrackAComposedVerifierPreservesScopeAndFrozenIntentBinding(t *testing.T) {
	receipt := providers.Receipt{Status: providers.ReceiptSuccess, ChainID: contracts.ChainIDArcMainnet, TransactionHash: mainnetVerifierHash, BlockNumber: 10, BlockHash: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Confirmations: 1}
	verifier, repository, value, _, _ := mainnetComposedVerifier(t, intents.TypePayroll, receipt)
	if _, err := verifier.Verify(context.Background(), value, mainnetReference(t)); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("missing scope error=%v", err)
	}
	mismatched, err := approveIntent(t, frozenIntentVariant(t, intents.TypePayroll, "different-nonce"))
	if err != nil {
		t.Fatal(err)
	}
	repository.intent = mismatched
	scope, _ := storage.NewScope("tenant", "actor", "request", "trace")
	if _, err := verifier.Verify(storage.WithScope(context.Background(), scope), value, mainnetReference(t)); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("frozen intent binding error=%v", err)
	}
}
