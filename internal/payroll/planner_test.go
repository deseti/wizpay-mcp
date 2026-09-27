package payroll

import (
	"strings"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

func TestMainnetPayrollPlannerFailsClosedInTrackA(t *testing.T) {
	intent := mainnetPayrollIntent(t)
	_, err := NewPlanner(nil).Plan(intent)
	if err == nil || !strings.Contains(err.Error(), "disabled in Track A") {
		t.Fatalf("error = %v", err)
	}
}

func mainnetPayrollIntent(t *testing.T) intents.Intent {
	t.Helper()
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	tokenIn := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: contracts.AddressUSDCMainnet, Symbol: "USDC", Decimals: 6}
	tokenOut := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: contracts.AddressEURCMainnet, Symbol: "EURC", Decimals: 6}
	amount := intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}
	intent, err := intents.NewDraft(intents.Params{
		IntentID: "payroll-mainnet", Version: 1, ClientRequestID: "request-mainnet", Nonce: "nonce-mainnet", Type: intents.TypePayroll,
		Ownership: intents.Ownership{UserID: "user", IdentityProvider: "circle", ProviderUserReference: "provider-user", WalletBindingID: "binding", WalletBindingVersion: 1, WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet},
		Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{SchemaVersion: intents.FinancialSchemaPhase12, Variant: intents.PayrollVariantSingle, TokenIn: tokenIn, Recipients: []intents.Recipient{{Address: "0x3333333333333333333333333333333333333333", TokenOut: tokenOut, AmountIn: amount, MinAmountOut: amount}}, Total: amount, ReferenceID: "payroll-mainnet"}},
		Route:     intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferencePayroll, Version: intents.RouteVersionPayroll}, Constraints: intents.Constraints{Deadline: now.Add(20 * time.Minute), PolicyReference: "policy:1"}, CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return intent
}
