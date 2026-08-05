package swap

import (
	"bytes"
	"math/big"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractswap "github.com/deseti/wizpay-mcp/internal/contracts/swap"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

var plannerTestNow = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

func TestPlannerMapsFrozenSwapExactly(t *testing.T) {
	intent := swapIntent(t)
	digest := intent.Digest()
	first, err := (Planner{}).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	second, err := (Planner{}).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	call := first.EncodedCall()
	if first.IntentID() != intent.IntentID() || first.IntentDigest() != digest || first.Capability() != intents.TypeSwap {
		t.Fatalf("plan identity = %q/%q/%q", first.IntentID(), first.IntentDigest(), first.Capability())
	}
	if first.ContractID() != contracts.ContractWizPaySwapExecutor || first.RegistryVersion() != contracts.RegistryVersion {
		t.Fatalf("deployment binding = %q/%d", first.ContractID(), first.RegistryVersion())
	}
	if first.ChainID() != contracts.ChainIDArcTestnet || first.WalletAddress() != intent.Ownership().WalletAddress {
		t.Fatalf("ownership binding = %q/%q", first.ChainID(), first.WalletAddress())
	}
	if !contracts.AddressesEqual(call.To(), contracts.AddressWizPaySwapExecutor) {
		t.Fatalf("target = %q", call.To())
	}
	if call.Function() != contractswap.SigExecuteSwap || call.Selector() != contracts.Selector4(contractswap.SigExecuteSwap) {
		t.Fatalf("function/selector = %q/%x", call.Function(), call.Selector())
	}
	if !bytes.Equal(call.CallData(), second.EncodedCall().CallData()) {
		t.Fatal("repeated planning produced different calldata")
	}

	method, err := contractswap.MethodBySignature(contractswap.SigExecuteSwap)
	if err != nil {
		t.Fatal(err)
	}
	values, err := method.Inputs.Unpack(call.CallData()[4:])
	if err != nil {
		t.Fatal(err)
	}
	financial := intent.Financial().Swap
	assertAddress(t, values[0], financial.Router)
	assertAddress(t, values[1], financial.InputToken.Address)
	assertAddress(t, values[2], financial.OutputToken.Address)
	assertBigInt(t, values[3], financial.InputAmount.BaseUnits)
	assertBigInt(t, values[4], financial.MinimumOutput.BaseUnits)
	assertAddress(t, values[5], financial.Recipient)
	assertBigInt(t, values[6], big.NewInt(financial.Deadline.UTC().Unix()).String())
	if intent.Digest() != digest {
		t.Fatal("planning mutated intent digest")
	}
	mutated := call.CallData()
	mutated[0] ^= 0xff
	if bytes.Equal(mutated, first.EncodedCall().CallData()) {
		t.Fatal("calldata mutation escaped defensive copy")
	}
}

func TestPlannerExposesOnlyIntentPlanningSurface(t *testing.T) {
	method, ok := reflect.TypeOf(Planner{}).MethodByName("Plan")
	if !ok || method.Type.NumIn() != 2 || method.Type.In(1) != reflect.TypeOf(intents.Intent{}) {
		t.Fatalf("unsafe Plan signature: %v", method.Type)
	}
}

func TestPlannerRejectsNonExecutableOrCorruptedIntents(t *testing.T) {
	tests := []struct {
		name   string
		intent func(*testing.T) intents.Intent
	}{
		{"wrong_type", func(t *testing.T) intents.Intent { return legacyPayrollIntent(t) }},
		{"legacy", func(t *testing.T) intents.Intent { return legacySwapIntent(t) }},
		{"schema_0_mixed", corruptSwap(func(params reflect.Value) {
			params.FieldByName("Financial").FieldByName("Swap").Elem().FieldByName("SchemaVersion").SetUint(0)
		})},
		{"incomplete_phase12", corruptSwap(func(params reflect.Value) {
			params.FieldByName("Financial").FieldByName("Swap").Elem().FieldByName("Quote").Set(reflect.Zero(reflect.TypeOf((*intents.SwapQuote)(nil))))
		})},
		{"wrong_route_type", corruptSwap(func(params reflect.Value) {
			params.FieldByName("Route").FieldByName("Type").SetString(string(intents.RouteApprovedProvider))
		})},
		{"wrong_route_reference", corruptSwap(func(params reflect.Value) {
			params.FieldByName("Route").FieldByName("Reference").SetString("OTHER")
		})},
		{"wrong_route_version", corruptSwap(func(params reflect.Value) {
			params.FieldByName("Route").FieldByName("Version").SetUint(2)
		})},
		{"wrong_chain", corruptSwap(func(params reflect.Value) {
			params.FieldByName("Ownership").FieldByName("ChainID").SetString("1")
		})},

		{"zero_deadline", corruptSwap(func(params reflect.Value) {
			params.FieldByName("Financial").FieldByName("Swap").Elem().FieldByName("Deadline").Set(reflect.ValueOf(time.Time{}))
		})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := (Planner{}).Plan(tt.intent(t)); err == nil {
				t.Fatal("expected planner rejection")
			}
		})
	}
	if _, err := NewPlanner(contracts.NewRegistry()).Plan(swapIntent(t)); err == nil {
		t.Fatal("unregistered deployment must be rejected")
	}
}

func corruptSwap(mutate func(reflect.Value)) func(*testing.T) intents.Intent {
	return func(t *testing.T) intents.Intent {
		intent := swapIntent(t)
		params := reflect.ValueOf(&intent).Elem().FieldByName("params")
		writable := reflect.NewAt(params.Type(), unsafe.Pointer(params.UnsafeAddr())).Elem()
		mutate(writable)
		return intent
	}
}

func swapIntent(t *testing.T) intents.Intent {
	t.Helper()
	input := plannerToken("0x1111111111111111111111111111111111111111", "USDC")
	output := plannerToken("0x5555555555555555555555555555555555555555", "EURC")
	expected := plannerAmount("9.5", "9500000")
	minimum := plannerAmount("9.405", "9405000")
	deadline := plannerTestNow.Add(10 * time.Minute)
	params := intents.Params{
		IntentID: "int_swap_plan", Version: 1, ClientRequestID: "req_swap_plan", Nonce: "nonce_swap_plan", Type: intents.TypeSwap,
		Ownership: plannerOwnership(),
		Financial: intents.FinancialParameters{Swap: &intents.SwapParameters{
			SchemaVersion: intents.FinancialSchemaPhase12, InputToken: input, OutputToken: output,
			InputAmount: plannerAmount("10", "10000000"), ExpectedOutput: expected, MinimumOutput: minimum,
			MaxSlippageBPS: 100, Router: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Recipient: "0x2222222222222222222222222222222222222222",
			Quote: &intents.SwapQuote{QuoteID: "quote_plan_001", Source: "frozen-source", ExpectedAmountOut: expected,
				MinAmountOut: minimum, Router: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				ExpiresAt: plannerTestNow.Add(15 * time.Minute), EvidenceReference: "evidence_plan_001"},
			Deadline: deadline,
		}},
		Route:       intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferenceSwap, Version: intents.RouteVersionSwap},
		Constraints: intents.Constraints{Deadline: plannerTestNow.Add(20 * time.Minute), PolicyReference: "policy_v1"},
		CreatedAt:   plannerTestNow, ExpiresAt: plannerTestNow.Add(30 * time.Minute),
	}
	intent, err := intents.NewDraft(params)
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, plannerTestNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func freezeIntent(t *testing.T, params intents.Params) intents.Intent {
	t.Helper()
	intent, err := intents.NewDraft(params)
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, params.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func legacyPayrollIntent(t *testing.T) intents.Intent {
	t.Helper()
	token := plannerToken("0x1111111111111111111111111111111111111111", "USDC")
	return freezeIntent(t, intents.Params{
		IntentID: "legacy_payroll", Version: 1, ClientRequestID: "legacy_payroll_req", Nonce: "legacy_payroll_nonce", Type: intents.TypePayroll,
		Ownership: plannerOwnership(),
		Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{Token: token,
			Recipients: []intents.Recipient{{Address: "0x3333333333333333333333333333333333333333", Amount: plannerAmount("1", "1000000")}},
			Total:      plannerAmount("1", "1000000")}},
		Route:       intents.Route{Type: intents.RouteAllowlistedContract, Reference: "legacy_payroll_route", Version: 1},
		Constraints: intents.Constraints{Deadline: plannerTestNow.Add(20 * time.Minute), PolicyReference: "policy_v1"},
		CreatedAt:   plannerTestNow, ExpiresAt: plannerTestNow.Add(30 * time.Minute),
	})
}

func legacySwapIntent(t *testing.T) intents.Intent {
	t.Helper()
	return freezeIntent(t, intents.Params{
		IntentID: "legacy_swap", Version: 1, ClientRequestID: "legacy_swap_req", Nonce: "legacy_swap_nonce", Type: intents.TypeSwap,
		Ownership: plannerOwnership(),
		Financial: intents.FinancialParameters{Swap: &intents.SwapParameters{
			InputToken:  plannerToken("0x1111111111111111111111111111111111111111", "USDC"),
			OutputToken: plannerToken("0x5555555555555555555555555555555555555555", "EURC"),
			InputAmount: plannerAmount("10", "10000000"), ExpectedOutput: plannerAmount("9.5", "9500000"),
			MinimumOutput: plannerAmount("9", "9000000"), QuoteReference: "legacy_quote", MaxSlippageBPS: 100}},
		Route:       intents.Route{Type: intents.RouteApprovedProvider, Reference: "legacy_swap_route", Version: 1},
		Constraints: intents.Constraints{Deadline: plannerTestNow.Add(20 * time.Minute), PolicyReference: "policy_v1"},
		CreatedAt:   plannerTestNow, ExpiresAt: plannerTestNow.Add(30 * time.Minute),
	})
}

func plannerOwnership() intents.Ownership {
	return intents.Ownership{UserID: "user_001", IdentityProvider: "circle", ProviderUserReference: "provider_user_001",
		WalletBindingID: "bind_001", WalletBindingVersion: 4, WalletID: "wallet_001",
		WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcTestnet, Network: "arc-testnet"}
}

func plannerToken(address, symbol string) intents.Token {
	return intents.Token{ChainID: contracts.ChainIDArcTestnet, Standard: "ERC20", Address: address, Symbol: symbol, Decimals: 6}
}

func plannerAmount(decimal, baseUnits string) intents.Amount {
	return intents.Amount{Decimal: decimal, BaseUnits: baseUnits, Decimals: 6}
}

func assertAddress(t *testing.T, got any, want string) {
	t.Helper()
	address, ok := got.(common.Address)
	if !ok || !contracts.AddressesEqual(address.Hex(), want) {
		t.Fatalf("address = %v, want %s", got, want)
	}
}

func assertBigInt(t *testing.T, got any, want string) {
	t.Helper()
	value, ok := got.(*big.Int)
	if !ok || value.String() != want {
		t.Fatalf("integer = %v, want %s", got, want)
	}
}
