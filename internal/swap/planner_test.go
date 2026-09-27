package swap

import (
	"strings"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

func TestMainnetSwapPlannerFailsClosedInTrackA(t *testing.T) {
	intent := mainnetSwapIntent(t)
	_, err := NewPlanner(nil).Plan(intent)
	if err == nil || !strings.Contains(err.Error(), "disabled in Track A") {
		t.Fatalf("error = %v", err)
	}
}

func mainnetSwapIntent(t *testing.T) intents.Intent {
	t.Helper()
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	input := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: contracts.AddressEURCMainnet, Symbol: "EURC", Decimals: 6}
	output := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: contracts.AddressUSDCMainnet, Symbol: "USDC", Decimals: 6}
	inAmount := intents.Amount{Decimal: "10", BaseUnits: "10000000", Decimals: 6}
	outAmount := intents.Amount{Decimal: "9", BaseUnits: "9000000", Decimals: 6}
	owner := intents.Ownership{UserID: "user", IdentityProvider: "circle", ProviderUserReference: "provider-user", WalletBindingID: "binding", WalletBindingVersion: 1, WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet}
	intent, err := intents.NewDraft(intents.Params{IntentID: "swap-mainnet", Version: 1, ClientRequestID: "request-mainnet", Nonce: "nonce-mainnet", Type: intents.TypeSwap, Ownership: owner,
		Financial: intents.FinancialParameters{Swap: &intents.SwapParameters{SchemaVersion: intents.FinancialSchemaPhase12, InputToken: input, OutputToken: output, InputAmount: inAmount, ExpectedOutput: outAmount, MinimumOutput: outAmount, MaxSlippageBPS: 100, Router: contracts.AddressUniswapUniversalRouter, Recipient: owner.WalletAddress, Quote: &intents.SwapQuote{QuoteID: "quote", Source: "reviewed", ExpectedAmountOut: outAmount, MinAmountOut: outAmount, Router: contracts.AddressUniswapUniversalRouter, ExpiresAt: now.Add(15 * time.Minute), EvidenceReference: "evidence"}, Deadline: now.Add(10 * time.Minute)}},
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
