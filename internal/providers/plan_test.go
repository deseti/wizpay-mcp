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
				ChainID:       contracts.ChainIDArcTestnet, Network: contracts.NetworkArcTestnet,
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

func TestNewContractExecutionPlanRejectsMissingFreshness(t *testing.T) {
	_, err := providers.NewContractExecutionPlan(providers.ContractExecutionParams{
		WalletBindingID: "binding-test", WalletID: "wallet-test",
		WalletAddress: "0x2222222222222222222222222222222222222222",
		ChainID:       contracts.ChainIDArcTestnet, Network: contracts.NetworkArcTestnet,
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
		ChainID:       contracts.ChainIDArcTestnet, Network: contracts.NetworkArcTestnet,
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
	call, err := payroll.EncodeRouteAndPay(nil, payroll.SinglePayment{
		TokenIn: "0x1111111111111111111111111111111111111111", TokenOut: "0x5555555555555555555555555555555555555555",
		AmountIn: big.NewInt(1), MinAmountOut: big.NewInt(1),
		Recipient: "0x3333333333333333333333333333333333333333",
	})
	if err != nil {
		t.Fatal(err)
	}
	return call
}

func mustSwapCall(t *testing.T) contracts.EncodedCall {
	t.Helper()
	call, err := swap.EncodeExecuteSwap(nil, swap.ExecuteSwapInput{
		Router:  "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		TokenIn: "0x1111111111111111111111111111111111111111", TokenOut: "0x5555555555555555555555555555555555555555",
		AmountIn: big.NewInt(10), MinAmountOut: big.NewInt(9),
		Recipient: "0x2222222222222222222222222222222222222222", Deadline: planTestNow.Add(15 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return call
}
