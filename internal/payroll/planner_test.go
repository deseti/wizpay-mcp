package payroll

import (
	"bytes"
	"math/big"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractpayroll "github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

var plannerTestNow = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

func TestPlannerMapsSingleIntentToRouteAndPay(t *testing.T) {
	intent := payrollIntent(t, intents.PayrollVariantSingle)
	digest := intent.Digest()

	plan, err := (Planner{}).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	call := plan.EncodedCall()
	if plan.IntentID() != intent.IntentID() || plan.IntentDigest() != digest {
		t.Fatalf("plan identity = %q/%q", plan.IntentID(), plan.IntentDigest())
	}
	if plan.Capability() != intents.TypePayroll {
		t.Fatalf("capability = %q", plan.Capability())
	}
	if plan.ContractID() != contracts.ContractWizPayPayroll || plan.RegistryVersion() != contracts.RegistryVersion {
		t.Fatalf("deployment binding = %q/%d", plan.ContractID(), plan.RegistryVersion())
	}
	if plan.ChainID() != contracts.ChainIDArcTestnet || plan.WalletAddress() != intent.Ownership().WalletAddress {
		t.Fatalf("ownership binding = %q/%q", plan.ChainID(), plan.WalletAddress())
	}
	if !contracts.AddressesEqual(call.To(), contracts.AddressWizPayPayroll) {
		t.Fatalf("target = %q", call.To())
	}
	if call.Function() != contractpayroll.SigRouteAndPay || call.Selector() != contracts.Selector4(contractpayroll.SigRouteAndPay) {
		t.Fatalf("function/selector = %q/%x", call.Function(), call.Selector())
	}
	if bytes.Contains(call.CallData(), []byte("single-reference-not-calldata")) {
		t.Fatal("SINGLE reference ID entered routeAndPay calldata")
	}

	method, err := contractpayroll.MethodBySignature(contractpayroll.SigRouteAndPay)
	if err != nil {
		t.Fatal(err)
	}
	values, err := method.Inputs.Unpack(call.CallData()[4:])
	if err != nil {
		t.Fatal(err)
	}
	financial := intent.Financial().Payroll
	line := financial.Recipients[0]
	assertAddress(t, values[0], financial.TokenIn.Address)
	assertAddress(t, values[1], line.TokenOut.Address)
	assertBigInt(t, values[2], line.AmountIn.BaseUnits)
	assertBigInt(t, values[3], line.MinAmountOut.BaseUnits)
	assertAddress(t, values[4], line.Address)
	if intent.Digest() != digest {
		t.Fatal("planning mutated the intent digest")
	}
}

func TestPlannerMapsBatchVariantsWithoutReordering(t *testing.T) {
	tests := []struct {
		name      string
		variant   intents.PayrollVariant
		signature string
	}{
		{"single_token_out", intents.PayrollVariantBatchSingleTokenOut, contractpayroll.SigBatchSingleTokenOut},
		{"multi_token_out", intents.PayrollVariantBatchMultiTokenOut, contractpayroll.SigBatchMultiTokenOut},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := payrollIntent(t, tt.variant)
			first, err := (Planner{}).Plan(intent)
			if err != nil {
				t.Fatal(err)
			}
			second, err := (Planner{}).Plan(intent)
			if err != nil {
				t.Fatal(err)
			}
			call := first.EncodedCall()
			if call.Function() != tt.signature || call.Selector() != contracts.Selector4(tt.signature) {
				t.Fatalf("function/selector = %q/%x", call.Function(), call.Selector())
			}
			if !bytes.Equal(call.CallData(), second.EncodedCall().CallData()) {
				t.Fatal("repeated planning produced different calldata")
			}
			method, err := contractpayroll.MethodBySignature(tt.signature)
			if err != nil {
				t.Fatal(err)
			}
			values, err := method.Inputs.Unpack(call.CallData()[4:])
			if err != nil {
				t.Fatal(err)
			}
			financial := intent.Financial().Payroll
			assertAddress(t, values[0], financial.TokenIn.Address)
			const recipientsIndex = 2
			if tt.variant == intents.PayrollVariantBatchMultiTokenOut {
				tokenOuts := values[1].([]common.Address)
				for i, line := range financial.Recipients {
					assertAddress(t, tokenOuts[i], line.TokenOut.Address)
				}
			} else {
				assertAddress(t, values[1], financial.Recipients[0].TokenOut.Address)
			}
			recipients := values[recipientsIndex].([]common.Address)
			amountsIn := values[recipientsIndex+1].([]*big.Int)
			minAmountsOut := values[recipientsIndex+2].([]*big.Int)
			for i, line := range financial.Recipients {
				assertAddress(t, recipients[i], line.Address)
				assertBigInt(t, amountsIn[i], line.AmountIn.BaseUnits)
				assertBigInt(t, minAmountsOut[i], line.MinAmountOut.BaseUnits)
			}
			if got := values[recipientsIndex+3].(string); got != financial.ReferenceID {
				t.Fatalf("reference ID = %q, want %q", got, financial.ReferenceID)
			}
			mutated := call.CallData()
			mutated[0] ^= 0xff
			if bytes.Equal(mutated, first.EncodedCall().CallData()) {
				t.Fatal("calldata mutation escaped defensive copy")
			}
		})
	}
}

func TestPlannerExposesOnlyIntentPlanningSurface(t *testing.T) {
	method, ok := reflect.TypeOf(Planner{}).MethodByName("Plan")
	if !ok {
		t.Fatal("Plan method missing")
	}
	if method.Type.NumIn() != 2 || method.Type.In(1) != reflect.TypeOf(intents.Intent{}) {
		t.Fatalf("Plan signature accepts override surface: %s", method.Type)
	}
}

func TestPlannerRejectsNonExecutableOrCorruptedIntents(t *testing.T) {
	valid := payrollIntent(t, intents.PayrollVariantSingle)
	tests := []struct {
		name   string
		intent func(*testing.T) intents.Intent
	}{
		{"wrong_type", func(t *testing.T) intents.Intent { return legacySwapIntent(t) }},
		{"legacy", func(t *testing.T) intents.Intent { return legacyPayrollIntent(t) }},
		{"schema_0_mixed", corruptPayroll(func(params reflect.Value) {
			params.FieldByName("Financial").FieldByName("Payroll").Elem().FieldByName("SchemaVersion").SetUint(0)
		})},
		{"incomplete_phase12", corruptPayroll(func(params reflect.Value) {
			payroll := params.FieldByName("Financial").FieldByName("Payroll").Elem()
			payroll.FieldByName("Recipients").Index(0).FieldByName("MinAmountOut").Set(reflect.ValueOf(intents.Amount{}))
		})},
		{"wrong_route_type", corruptPayroll(func(params reflect.Value) {
			params.FieldByName("Route").FieldByName("Type").SetString(string(intents.RouteApprovedProvider))
		})},
		{"wrong_route_reference", corruptPayroll(func(params reflect.Value) {
			params.FieldByName("Route").FieldByName("Reference").SetString("OTHER")
		})},
		{"wrong_route_version", corruptPayroll(func(params reflect.Value) {
			params.FieldByName("Route").FieldByName("Version").SetUint(2)
		})},
		{"wrong_chain", corruptPayroll(func(params reflect.Value) {
			params.FieldByName("Ownership").FieldByName("ChainID").SetString("1")
		})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := (Planner{}).Plan(tt.intent(t)); err == nil {
				t.Fatal("expected planner rejection")
			}
		})
	}
	if _, err := NewPlanner(contracts.NewRegistry()).Plan(valid); err == nil {
		t.Fatal("unregistered deployment must be rejected")
	}
}

func corruptPayroll(mutate func(reflect.Value)) func(*testing.T) intents.Intent {
	return func(t *testing.T) intents.Intent {
		return corruptIntent(t, payrollIntent(t, intents.PayrollVariantSingle), mutate)
	}
}

func corruptIntent(t *testing.T, intent intents.Intent, mutate func(reflect.Value)) intents.Intent {
	t.Helper()
	params := reflect.ValueOf(&intent).Elem().FieldByName("params")
	writable := reflect.NewAt(params.Type(), unsafe.Pointer(params.UnsafeAddr())).Elem()
	mutate(writable)
	return intent
}

func payrollIntent(t *testing.T, variant intents.PayrollVariant) intents.Intent {
	t.Helper()
	tokenIn := plannerToken("0x1111111111111111111111111111111111111111", "USDC")
	tokenOutA := plannerToken("0x5555555555555555555555555555555555555555", "EURC")
	tokenOutB := plannerToken("0x6666666666666666666666666666666666666666", "USDX")
	lines := []intents.Recipient{
		{Address: "0x3333333333333333333333333333333333333333", TokenOut: tokenOutA, AmountIn: plannerAmount("1.25", "1250000"), MinAmountOut: plannerAmount("1.2", "1200000")},
		{Address: "0x4444444444444444444444444444444444444444", TokenOut: tokenOutA, AmountIn: plannerAmount("2", "2000000"), MinAmountOut: plannerAmount("1.9", "1900000")},
	}
	referenceID := "payroll-batch-001"
	total := plannerAmount("3.25", "3250000")
	if variant == intents.PayrollVariantSingle {
		lines = lines[:1]
		referenceID = "single-reference-not-calldata"
		total = plannerAmount("1.25", "1250000")
	}
	if variant == intents.PayrollVariantBatchMultiTokenOut {
		lines[1].TokenOut = tokenOutB
	}
	params := intents.Params{
		IntentID: "int_payroll_plan", Version: 1, ClientRequestID: "req_payroll_plan", Nonce: "nonce_payroll_plan", Type: intents.TypePayroll,
		Ownership: plannerOwnership(),
		Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{
			SchemaVersion: intents.FinancialSchemaPhase12, Variant: variant, TokenIn: tokenIn,
			Recipients: lines, Total: total, ReferenceID: referenceID,
		}},
		Route:       intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferencePayroll, Version: intents.RouteVersionPayroll},
		Constraints: intents.Constraints{Deadline: plannerTestNow.Add(20 * time.Minute), PolicyReference: "policy_v1"},
		CreatedAt:   plannerTestNow, ExpiresAt: plannerTestNow.Add(30 * time.Minute),
	}
	return freezeIntent(t, params)
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
	return intents.Ownership{
		UserID: "user_001", IdentityProvider: "circle", ProviderUserReference: "provider_user_001",
		WalletBindingID: "bind_001", WalletBindingVersion: 4, WalletID: "wallet_001",
		WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcTestnet, Network: "arc-testnet",
	}
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
