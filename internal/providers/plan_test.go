package providers_test

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	"github.com/deseti/wizpay-mcp/internal/contracts/swap"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

var planTestNow = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

func TestTokenTransferPlanValidateUnchanged(t *testing.T) {
	plan := providers.Plan{
		WalletBindingID: "binding-test", WalletID: "wallet-test",
		WalletAddress: "0x2222222222222222222222222222222222222222",
		ChainID:       "5042002", Network: "TESTNET",
		DestinationAddress: "0x3333333333333333333333333333333333333333",
		TokenID:            "token-test", Amount: "1",
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("transfer plan: %v", err)
	}
	if plan.EffectiveKind() != providers.PlanKindTokenTransfer {
		t.Fatalf("kind = %q", plan.EffectiveKind())
	}
	if _, ok := plan.EncodedCall(); ok {
		t.Fatal("transfer plan must not expose an encoded call")
	}
}

func TestNewContractExecutionPlanPayrollAndSwap(t *testing.T) {
	bound := planTestNow.Add(10 * time.Minute)
	for _, tc := range []struct {
		name string
		call contracts.EncodedCall
		id   contracts.ContractID
	}{
		{"payroll", mustPayrollCall(t), contracts.ContractWizPayPayroll},
		{"swap", mustSwapCall(t), contracts.ContractWizPaySwapExecutor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := providers.NewContractExecutionPlan(providers.ContractExecutionParams{
				WalletBindingID: "binding-test", WalletID: "wallet-test",
				WalletAddress: "0x2222222222222222222222222222222222222222",
				ChainID:       contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet,
				Call: tc.call, SubmitNotAfter: bound,
			})
			if err != nil {
				t.Fatal(err)
			}
			if plan.EffectiveKind() != providers.PlanKindContractExecution {
				t.Fatalf("kind = %q", plan.EffectiveKind())
			}
			got, ok := plan.EncodedCall()
			if !ok {
				t.Fatal("encoded call missing")
			}
			if got.ContractID() != tc.id {
				t.Fatalf("contract = %q", got.ContractID())
			}
			if !contracts.AddressesEqual(got.To(), tc.call.To()) {
				t.Fatalf("To = %q", got.To())
			}
			if plan.FreshnessExpired(planTestNow) {
				t.Fatal("fresh plan must not be expired")
			}
			if !plan.FreshnessExpired(bound) {
				t.Fatal("at bound must be expired (exclusive)")
			}
		})
	}
}

func TestSwapContractExecutionPlanBindsExactNativeFunding(t *testing.T) {
	for _, tc := range []struct{ name, tokenIn, tokenOut, want string }{
		{"USDC_to_EURC", contracts.AddressUSDCMainnet, contracts.AddressEURCMainnet, "1000000000000000000"},
		{"EURC_to_USDC", contracts.AddressEURCMainnet, contracts.AddressUSDCMainnet, "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call, err := swap.EncodeExecuteSwap(nil, swap.ExecuteSwapInput{TokenIn: tc.tokenIn, TokenOut: tc.tokenOut, AmountIn: big.NewInt(1_000_000), MinAmountOut: big.NewInt(900_000), MinHopPriceX36: big.NewInt(1), Deadline: big.NewInt(1)})
			if err != nil {
				t.Fatal(err)
			}
			plan, err := providers.NewContractExecutionPlan(providers.ContractExecutionParams{WalletBindingID: "binding", WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet, Call: call, SubmitNotAfter: planTestNow.Add(time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			got, ok := plan.NativeValueBaseUnits()
			if !ok || got != tc.want {
				t.Fatalf("native value=%q,%v want %q", got, ok, tc.want)
			}
			_, err = providers.NewContractExecutionPlan(providers.ContractExecutionParams{WalletBindingID: "binding", WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet, Call: call, SubmitNotAfter: planTestNow.Add(time.Minute), NativeValueBaseUnits: "1"})
			if err == nil {
				t.Fatal("wrong caller-specified native value must fail")
			}
		})
	}
}

func TestCrossTokenPayrollPlanBindsExactNativeFunding(t *testing.T) {
	for _, tc := range []struct{ name, in, out, want string }{{"USDC", contracts.AddressUSDCMainnet, contracts.AddressEURCMainnet, "1000000000000000000"}, {"EURC", contracts.AddressEURCMainnet, contracts.AddressUSDCMainnet, "0"}} {
		t.Run(tc.name, func(t *testing.T) {
			call, err := payroll.EncodeCrossTokenPayroll(nil, payroll.CrossTokenPayrollInput{TokenIn: tc.in, TokenOut: tc.out, Recipients: []string{"0x3333333333333333333333333333333333333333"}, OutputAmounts: []*big.Int{big.NewInt(900_000)}, GrossInput: big.NewInt(1_000_000), MinTotalOut: big.NewInt(900_000), MinHopPriceX36: big.NewInt(1), Deadline: big.NewInt(1), ReferenceID: "reference"})
			if err != nil {
				t.Fatal(err)
			}
			plan, err := providers.NewContractExecutionPlan(providers.ContractExecutionParams{WalletBindingID: "binding", WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet, Call: call, SubmitNotAfter: planTestNow.Add(time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			got, ok := plan.NativeValueBaseUnits()
			if !ok || got != tc.want {
				t.Fatalf("value=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestNewContractExecutionPlanRejectsMissingFreshness(t *testing.T) {
	_, err := providers.NewContractExecutionPlan(providers.ContractExecutionParams{
		WalletBindingID: "binding-test", WalletID: "wallet-test",
		WalletAddress: "0x2222222222222222222222222222222222222222",
		ChainID:       contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet,
		Call: mustPayrollCall(t),
	})
	if err == nil {
		t.Fatal("missing freshness must fail")
	}
}

func TestContractExecutionPlanRejectsTransferFields(t *testing.T) {
	plan, err := providers.NewContractExecutionPlan(providers.ContractExecutionParams{
		WalletBindingID: "binding-test", WalletID: "wallet-test",
		WalletAddress: "0x2222222222222222222222222222222222222222",
		ChainID:       contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet,
		Call: mustPayrollCall(t), SubmitNotAfter: planTestNow.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	plan.DestinationAddress = "0x3333333333333333333333333333333333333333"
	if err := plan.Validate(); err == nil {
		t.Fatal("mixed transfer fields on contract plan must fail")
	}
}

func TestContractExecutionPlanHasNoPublicCalldataOverrideFields(t *testing.T) {
	typ := reflect.TypeOf(providers.Plan{})
	forbidden := map[string]bool{
		"ContractAddress": true, "CallData": true, "Calldata": true,
		"Function": true, "Selector": true, "AbiFunctionSignature": true,
		"AbiParameters": true,
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if forbidden[field.Name] {
			t.Fatalf("Plan must not expose override field %s", field.Name)
		}
	}
	// Encoded call material must remain unexported.
	for _, name := range []string{"encodedCall", "hasEncodedCall", "submitNotAfter"} {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("expected unexported field %s", name)
		}
		if field.PkgPath == "" {
			t.Fatalf("field %s must be unexported", name)
		}
	}
}

func TestEarliestDeadline(t *testing.T) {
	a := planTestNow.Add(5 * time.Minute)
	b := planTestNow.Add(2 * time.Minute)
	c := planTestNow.Add(10 * time.Minute)
	got := providers.EarliestDeadline(time.Time{}, a, b, c)
	if !got.Equal(b) {
		t.Fatalf("earliest = %s want %s", got, b)
	}
	if !providers.EarliestDeadline().IsZero() {
		t.Fatal("no candidates must yield zero")
	}
}

func mustPayrollCall(t *testing.T) contracts.EncodedCall {
	t.Helper()
	call, err := payroll.EncodeSameTokenPayroll(nil, payroll.SameTokenPayrollInput{
		Token: contracts.AddressUSDCMainnet, Recipients: []string{"0x3333333333333333333333333333333333333333"},
		Amounts: []*big.Int{big.NewInt(1)}, ReferenceID: "plan-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return call
}

func mustSwapCall(t *testing.T) contracts.EncodedCall {
	t.Helper()
	call, err := swap.EncodeExecuteSwap(nil, swap.ExecuteSwapInput{
		TokenIn: contracts.AddressEURCMainnet, TokenOut: contracts.AddressUSDCMainnet,
		AmountIn: big.NewInt(10), MinAmountOut: big.NewInt(9),
		MinHopPriceX36: big.NewInt(1), Deadline: big.NewInt(planTestNow.Add(15 * time.Minute).Unix()),
	})
	if err != nil {
		t.Fatal(err)
	}
	return call
}
