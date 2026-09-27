package policies

import (
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

func TestTrackDSwapPolicyUsesGrossInputExposure(t *testing.T) {
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	owner := intents.Ownership{UserID: "user", IdentityProvider: "circle", ProviderUserReference: "provider-user", WalletBindingID: "binding", WalletBindingVersion: 1, WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet}
	input := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: contracts.AddressUSDCMainnet, Symbol: "USDC", Decimals: 6}
	output := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: contracts.AddressEURCMainnet, Symbol: "EURC", Decimals: 6}
	gross := intents.Amount{Decimal: "10", BaseUnits: "10000000", Decimals: 6}
	expected := intents.Amount{Decimal: "9", BaseUnits: "9000000", Decimals: 6}
	minimum := intents.Amount{Decimal: "8.91", BaseUnits: "8910000", Decimals: 6}
	intent, err := intents.NewDraft(intents.Params{IntentID: "swap", Version: 1, ClientRequestID: "request", Nonce: "nonce", Type: intents.TypeSwap, Ownership: owner,
		Financial: intents.FinancialParameters{Swap: &intents.SwapParameters{SchemaVersion: intents.FinancialSchemaPhase12, InputToken: input, OutputToken: output, InputAmount: gross, ExpectedOutput: expected, MinimumOutput: minimum, MaxSlippageBPS: 100, MinHopPriceX36: "1", Router: contracts.AddressUniswapUniversalRouter, Recipient: owner.WalletAddress, Quote: &intents.SwapQuote{QuoteID: "quote", Source: "source", ExpectedAmountOut: expected, MinAmountOut: minimum, Router: contracts.AddressUniswapUniversalRouter, ExpiresAt: now.Add(15 * time.Minute), EvidenceReference: "evidence"}, Deadline: now.Add(10 * time.Minute)}},
		Route:     intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferenceSwap, Version: intents.RouteVersionSwap}, Constraints: intents.Constraints{Deadline: now.Add(20 * time.Minute), PolicyReference: "policy:1"}, CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	view, err := newIntentView(intent)
	if err != nil {
		t.Fatal(err)
	}
	if view.spendAmount != gross || view.spendToken.Address != contracts.NormalizeAddress(contracts.AddressUSDCMainnet) {
		t.Fatalf("policy exposure=%#v token=%#v", view.spendAmount, view.spendToken)
	}
}
