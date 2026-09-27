package payroll

import (
	"math/big"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractpayroll "github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

const payrollTestHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func mainnetPayrollPlan(t *testing.T, intentOverride ...string) Plan {
	t.Helper()
	intent := mainnetPayrollIntent(t)
	call, err := contractpayroll.EncodeSameTokenPayroll(nil, contractpayroll.SameTokenPayrollInput{
		Token: contracts.AddressUSDCMainnet, Recipients: []string{"0x3333333333333333333333333333333333333333"},
		Amounts: []*big.Int{big.NewInt(1_000_000)}, ReferenceID: "payroll-mainnet",
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := newPlan(intent, call)
	if len(intentOverride) > 0 {
		plan.intentID = intentOverride[0]
	}
	return plan
}

func TestMainnetPayrollVerifierCannotCompleteInTrackA(t *testing.T) {
	intent := mainnetPayrollIntent(t)
	result, err := NewVerifier(nil).Verify(intent, mainnetPayrollPlan(t), providers.Receipt{
		Status: providers.ReceiptSuccess, ChainID: contracts.ChainIDArcMainnet, TransactionHash: payrollTestHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != DomainUnverified || result.FinancialComplete() || result.DefinitiveFailure() || result.ReasonCode != "ARC_MAINNET_PAYROLL_EXECUTION_DISABLED" {
		t.Fatalf("result = %#v", result)
	}
}

func TestMainnetPayrollVerifierPreservesBindings(t *testing.T) {
	intent := mainnetPayrollIntent(t)
	if _, err := NewVerifier(nil).Verify(intent, Plan{}, providers.Receipt{}); err == nil {
		t.Fatal("empty plan must fail binding validation")
	}
	if _, err := NewVerifier(nil).Verify(intent, mainnetPayrollPlan(t, "other-intent"), providers.Receipt{}); err == nil {
		t.Fatal("intent binding mismatch must be rejected")
	}
	wrongSender := mainnetPayrollPlan(t)
	wrongSender.walletAddress = "0x9999999999999999999999999999999999999999"
	if _, err := NewVerifier(nil).Verify(intent, wrongSender, providers.Receipt{}); err == nil {
		t.Fatal("sender binding mismatch must be rejected")
	}
	result, err := NewVerifier(nil).Verify(intent, mainnetPayrollPlan(t), providers.Receipt{Status: providers.ReceiptSuccess, ChainID: "5042002", TransactionHash: payrollTestHash})
	if err != nil || result.Status != DomainFailed || result.ReasonCode != "RECEIPT_CHAIN_MISMATCH" {
		t.Fatalf("wrong-chain result=%#v err=%v", result, err)
	}
}
