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

func TestTrackFNativeValueFundingMatrix(t *testing.T) {
	const exact = "1000000000000000000"
	const oneLow = "999999999999999999"
	const oneHigh = "1000000000000000001"

	sameToken := mustPayrollCall(t)
	usdcSwap, err := swap.EncodeExecuteSwap(nil, swap.ExecuteSwapInput{TokenIn: contracts.AddressUSDCMainnet, TokenOut: contracts.AddressEURCMainnet, AmountIn: big.NewInt(1_000_000), MinAmountOut: big.NewInt(900_000), MinHopPriceX36: big.NewInt(1), Deadline: big.NewInt(1)})
	if err != nil {
		t.Fatal(err)
	}
	eurcSwap, err := swap.EncodeExecuteSwap(nil, swap.ExecuteSwapInput{TokenIn: contracts.AddressEURCMainnet, TokenOut: contracts.AddressUSDCMainnet, AmountIn: big.NewInt(1_000_000), MinAmountOut: big.NewInt(900_000), MinHopPriceX36: big.NewInt(1), Deadline: big.NewInt(1)})
	if err != nil {
		t.Fatal(err)
	}
	usdcPayroll := mustCrossTokenPayrollCall(t, contracts.AddressUSDCMainnet, contracts.AddressEURCMainnet)
	eurcPayroll := mustCrossTokenPayrollCall(t, contracts.AddressEURCMainnet, contracts.AddressUSDCMainnet)

	for _, tc := range []struct {
		name string
		call contracts.EncodedCall
		want string
	}{
		{"same_token_payroll", sameToken, "0"},
		{"swap_USDC_to_EURC", usdcSwap, exact},
		{"swap_EURC_to_USDC", eurcSwap, "0"},
		{"cross_token_payroll_USDC_to_EURC", usdcPayroll, exact},
		{"cross_token_payroll_EURC_to_USDC", eurcPayroll, "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			build := func(value string) (providers.Plan, error) {
				return providers.NewContractExecutionPlan(providers.ContractExecutionParams{
					WalletBindingID: "binding", WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222",
					ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet,
					Call: tc.call, SubmitNotAfter: planTestNow.Add(time.Minute), NativeValueBaseUnits: value,
				})
			}
			plan, err := build(tc.want)
			if err != nil {
				t.Fatalf("exact funding rejected: %v", err)
			}
			if got, ok := plan.NativeValueBaseUnits(); !ok || got != tc.want {
				t.Fatalf("native value = %q,%t want %q,true", got, ok, tc.want)
			}

			wrong := []string{"-1", "01", "not-a-number", new(big.Int).Lsh(big.NewInt(1), 257).String()}
			if tc.want == exact {
				wrong = append(wrong, "0", oneLow, oneHigh)
			} else {
				wrong = append(wrong, "1")
			}
			for _, value := range wrong {
				if _, err := build(value); err == nil {
					t.Errorf("wrong native value %q accepted", value)
				}
			}
		})
	}

	sendPlan, err := providers.NewTokenTransferPlan(providers.TokenTransferParams{
		WalletBindingID: "binding", WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222",
		ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet,
		Destination: "0x3333333333333333333333333333333333333333", TokenID: "USDC",
		TokenAddress: contracts.AddressUSDCMainnet, TokenDecimals: 6, Amount: "1", AmountBaseUnits: "1000000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sendPlan.NativeValueBaseUnits(); ok {
		t.Fatal("SEND introduced native value")
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

func mustCrossTokenPayrollCall(t *testing.T, tokenIn, tokenOut string) contracts.EncodedCall {
	t.Helper()
	call, err := payroll.EncodeCrossTokenPayroll(nil, payroll.CrossTokenPayrollInput{
		TokenIn: tokenIn, TokenOut: tokenOut,
		Recipients: []string{"0x3333333333333333333333333333333333333333"}, OutputAmounts: []*big.Int{big.NewInt(900_000)},
		GrossInput: big.NewInt(1_000_000), MinTotalOut: big.NewInt(900_000), MinHopPriceX36: big.NewInt(1),
		Deadline: big.NewInt(1), ReferenceID: "track-f-reference",
	})
	if err != nil {
		t.Fatal(err)
	}
	return call
}

func FuzzTrackFNativeValueDerivationIsDeterministic(f *testing.F) {
	for _, seed := range []uint64{1, 1_000_000, 10_000_000, ^uint64(0)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, amount uint64) {
		if amount == 0 {
			t.Skip()
		}
		input := new(big.Int).SetUint64(amount)
		call, err := swap.EncodeExecuteSwap(nil, swap.ExecuteSwapInput{
			TokenIn: contracts.AddressUSDCMainnet, TokenOut: contracts.AddressEURCMainnet,
			AmountIn: input, MinAmountOut: big.NewInt(1), MinHopPriceX36: big.NewInt(1), Deadline: big.NewInt(1),
		})
		if err != nil {
			t.Fatal(err)
		}
		build := func() providers.Plan {
			plan, buildErr := providers.NewContractExecutionPlan(providers.ContractExecutionParams{
				WalletBindingID: "binding", WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222",
				ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet,
				Call: call, SubmitNotAfter: planTestNow.Add(time.Minute),
			})
			if buildErr != nil {
				t.Fatal(buildErr)
			}
			return plan
		}
		first, second := build(), build()
		firstValue, firstOK := first.NativeValueBaseUnits()
		secondValue, secondOK := second.NativeValueBaseUnits()
		want := new(big.Int).Mul(new(big.Int).Set(input), big.NewInt(1_000_000_000_000)).String()
		if !firstOK || !secondOK || firstValue != want || secondValue != want || firstValue != secondValue {
			t.Fatalf("native value is not deterministic: first=%q second=%q want=%q", firstValue, secondValue, want)
		}
	})
}
