package swap

import (
	"bytes"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractswap "github.com/deseti/wizpay-mcp/internal/contracts/swap"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

func TestMainnetSwapPlannerBuildsSealedCall(t *testing.T) {
	intent := mainnetSwapIntent(t)
	plan, err := NewPlanner(nil).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := contractswap.DecodeExecuteSwapCall(plan.EncodedCall().CallData())
	if err != nil {
		t.Fatal(err)
	}
	if plan.EncodedCall().To() != contracts.AddressWizPaySwapExecutor || decoded.AmountIn.String() != "10000000" || decoded.MinAmountOut.String() != "9000000" || decoded.MinHopPriceX36.String() != "1" || decoded.Deadline.Int64() != intent.Financial().Swap.Deadline.Unix() {
		t.Fatalf("wrong plan: %#v", decoded)
	}
	mutated := plan.EncodedCall().CallData()
	mutated[len(mutated)-1] ^= 1
	if bytes.Equal(mutated, plan.EncodedCall().CallData()) {
		t.Fatal("sealed calldata is mutable")
	}
}

func mainnetSwapIntent(t *testing.T) intents.Intent {
	return mainnetSwapIntentDirection(t, contracts.AddressEURCMainnet)
}

func mainnetSwapIntentDirection(t *testing.T, inputAddress string) intents.Intent {
	t.Helper()
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	inputSymbol, outputAddress, outputSymbol := "EURC", contracts.AddressUSDCMainnet, "USDC"
	if contracts.AddressesEqual(inputAddress, contracts.AddressUSDCMainnet) {
		inputSymbol, outputAddress, outputSymbol = "USDC", contracts.AddressEURCMainnet, "EURC"
	}
	input := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: inputAddress, Symbol: inputSymbol, Decimals: 6}
	output := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: outputAddress, Symbol: outputSymbol, Decimals: 6}
	inAmount := intents.Amount{Decimal: "10", BaseUnits: "10000000", Decimals: 6}
	outAmount := intents.Amount{Decimal: "9", BaseUnits: "9000000", Decimals: 6}
	owner := intents.Ownership{UserID: "user", IdentityProvider: "circle", ProviderUserReference: "provider-user", WalletBindingID: "binding", WalletBindingVersion: 1, WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet}
	intent, err := intents.NewDraft(intents.Params{IntentID: "swap-mainnet", Version: 1, ClientRequestID: "request-mainnet", Nonce: "nonce-mainnet", Type: intents.TypeSwap, Ownership: owner,
		Financial: intents.FinancialParameters{Swap: &intents.SwapParameters{SchemaVersion: intents.FinancialSchemaPhase12, InputToken: input, OutputToken: output, InputAmount: inAmount, ExpectedOutput: outAmount, MinimumOutput: outAmount, MaxSlippageBPS: 100, MinHopPriceX36: "1", Router: contracts.AddressUniswapUniversalRouter, Recipient: owner.WalletAddress, Quote: &intents.SwapQuote{QuoteID: "quote", Source: "reviewed", ExpectedAmountOut: outAmount, MinAmountOut: outAmount, Router: contracts.AddressUniswapUniversalRouter, ExpiresAt: now.Add(15 * time.Minute), EvidenceReference: "evidence"}, Deadline: now.Add(10 * time.Minute)}},
		Route:     intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferenceSwap, Version: intents.RouteVersionSwap}, Constraints: intents.Constraints{Deadline: now.Add(20 * time.Minute), PolicyReference: "policy:1"}, CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return intent
}
