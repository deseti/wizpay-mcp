package swap

import (
	"math/big"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractswap "github.com/deseti/wizpay-mcp/internal/contracts/swap"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

const swapTestHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func mainnetSwapPlan(t *testing.T, intentOverride ...string) Plan {
	t.Helper()
	intent := mainnetSwapIntent(t)
	call, err := contractswap.EncodeExecuteSwap(nil, contractswap.ExecuteSwapInput{
		TokenIn: contracts.AddressEURCMainnet, TokenOut: contracts.AddressUSDCMainnet,
		AmountIn: big.NewInt(10_000_000), MinAmountOut: big.NewInt(9_000_000),
		MinHopPriceX36: big.NewInt(1), Deadline: big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := newPlan(intent, call)
	if len(intentOverride) > 0 {
		plan.intentID = intentOverride[0]
	}
	return plan
}

func TestMainnetSwapVerifierCannotCompleteInTrackA(t *testing.T) {
	intent := mainnetSwapIntent(t)
	result, err := NewVerifier(nil).Verify(intent, mainnetSwapPlan(t), providers.Receipt{
		Status: providers.ReceiptSuccess, ChainID: contracts.ChainIDArcMainnet, TransactionHash: swapTestHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != DomainUnverified || result.FinancialComplete() || result.DefinitiveFailure() || result.ReasonCode != "ARC_MAINNET_SWAP_EXECUTION_DISABLED" {
		t.Fatalf("result = %#v", result)
	}
}

func TestMainnetSwapVerifierPreservesBindings(t *testing.T) {
	intent := mainnetSwapIntent(t)
	if _, err := NewVerifier(nil).Verify(intent, Plan{}, providers.Receipt{}); err == nil {
		t.Fatal("empty plan must fail binding validation")
	}
	if _, err := NewVerifier(nil).Verify(intent, mainnetSwapPlan(t, "other-intent"), providers.Receipt{}); err == nil {
		t.Fatal("intent binding mismatch must be rejected")
	}
	wrongSender := mainnetSwapPlan(t)
	wrongSender.walletAddress = "0x9999999999999999999999999999999999999999"
	if _, err := NewVerifier(nil).Verify(intent, wrongSender, providers.Receipt{}); err == nil {
		t.Fatal("sender binding mismatch must be rejected")
	}
	result, err := NewVerifier(nil).Verify(intent, mainnetSwapPlan(t), providers.Receipt{Status: providers.ReceiptSuccess, ChainID: "5042002", TransactionHash: swapTestHash})
	if err != nil || result.Status != DomainFailed || result.ReasonCode != "RECEIPT_CHAIN_MISMATCH" {
		t.Fatalf("wrong-chain result=%#v err=%v", result, err)
	}
}
