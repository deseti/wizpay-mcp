package policies

import (
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

func TestCrossTokenPayrollPolicyUsesGrossInputOnly(t *testing.T) {
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	in := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: contracts.AddressUSDCMainnet, Symbol: "USDC", Decimals: 6}
	out := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: contracts.AddressEURCMainnet, Symbol: "EURC", Decimals: 6}
	obligation := intents.Amount{Decimal: "10", BaseUnits: "10000000", Decimals: 6}
	gross := intents.Amount{Decimal: "11", BaseUnits: "11000000", Decimals: 6}
	owner := intents.Ownership{UserID: "user", IdentityProvider: "circle", ProviderUserReference: "provider", WalletBindingID: "binding", WalletBindingVersion: 1, WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet}
	intent, err := intents.NewDraft(intents.Params{IntentID: "cross", Version: 1, ClientRequestID: "request", Nonce: "nonce", Type: intents.TypePayroll, Ownership: owner, Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{SchemaVersion: intents.FinancialSchemaPhase12, Variant: intents.PayrollVariantSingle, TokenIn: in, Recipients: []intents.Recipient{{Address: "0x3333333333333333333333333333333333333333", TokenOut: out, MinAmountOut: obligation}}, Total: obligation, ReferenceID: "reference", CrossToken: &intents.CrossTokenPayrollParameters{GrossInput: gross, MinTotalOut: obligation, MinHopPriceX36: "1", Deadline: now.Add(10 * time.Minute)}}}, Route: intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferencePayroll, Version: intents.RouteVersionPayroll}, Constraints: intents.Constraints{Deadline: now.Add(20 * time.Minute), PolicyReference: "policy:1"}, CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	view, err := newIntentView(intent)
	if err != nil {
		t.Fatal(err)
	}
	if view.spendAmount != gross {
		t.Fatalf("exposure=%#v want gross %#v", view.spendAmount, gross)
	}
	// Track C remains fee-on-top and is covered independently.
	if payrollMaximumEmployerExposure(obligation).BaseUnits != "10100000" {
		t.Fatal("Track C fee-on-top exposure changed")
	}
}
