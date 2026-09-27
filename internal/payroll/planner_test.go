package payroll

import (
	"bytes"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractpayroll "github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

func TestMainnetPayrollPlannerBuildsSealedSameTokenCall(t *testing.T) {
	intent := mainnetPayrollIntent(t)
	plan, err := NewPlanner(nil).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	call := plan.EncodedCall()
	if call.To() != contracts.AddressWizPayPayroll || plan.WalletAddress() != intent.Ownership().WalletAddress {
		t.Fatalf("wrong binding: %#v", plan)
	}
	decoded, err := contractpayroll.DecodeSameTokenPayrollCall(call.CallData())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Token != contracts.AddressUSDCMainnet || len(decoded.Recipients) != 1 || decoded.Recipients[0] != "0x3333333333333333333333333333333333333333" || decoded.Amounts[0].String() != "1000000" || decoded.ReferenceID != "payroll-mainnet" {
		t.Fatalf("decoded = %#v", decoded)
	}
	mutated := call.CallData()
	mutated[len(mutated)-1] ^= 1
	if bytes.Equal(mutated, plan.EncodedCall().CallData()) {
		t.Fatal("plan calldata was mutable")
	}
}

func TestMainnetPayrollPlannerRejectsCrossToken(t *testing.T) {
	intent := mainnetPayrollIntentWithTokens(t, contracts.AddressUSDCMainnet, "USDC", contracts.AddressEURCMainnet, "EURC")
	if _, err := NewPlanner(nil).Plan(intent); err == nil {
		t.Fatal("cross-token payroll must fail closed")
	}
}

func mainnetPayrollIntent(t *testing.T) intents.Intent {
	return mainnetPayrollIntentWithTokens(t, contracts.AddressUSDCMainnet, "USDC", contracts.AddressUSDCMainnet, "USDC")
}

func mainnetPayrollIntentWithTokens(t *testing.T, inputAddress, inputSymbol, outputAddress, outputSymbol string) intents.Intent {
	t.Helper()
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	tokenIn := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: inputAddress, Symbol: inputSymbol, Decimals: 6}
	tokenOut := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: outputAddress, Symbol: outputSymbol, Decimals: 6}
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
