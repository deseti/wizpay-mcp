package payroll

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractpayroll "github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

const payrollTestHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func sameTokenFixture(t *testing.T, token, symbol string) (intents.Intent, Plan, providers.Receipt) {
	t.Helper()
	intent := mainnetPayrollIntentWithTokens(t, token, symbol, token, symbol)
	plan, err := NewPlanner(nil).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	refHash, digest := [32]byte{1}, [32]byte{2}
	employer := intent.Ownership().WalletAddress
	recipient := intent.Financial().Payroll.Recipients[0].Address
	logs := []providers.ReceiptLog{
		eventLog(t, contractpayroll.SigPayrollPayment, [][]byte{refHash[:], addressTopic(employer), addressTopic(token)}, common.HexToAddress(recipient), big.NewInt(0), big.NewInt(1_000_000)),
		eventLog(t, contractpayroll.SigPayrollReferenceConsumed, [][]byte{refHash[:], addressTopic(employer), addressTopic(token)}, common.HexToAddress(token), digest, big.NewInt(1_010_000), big.NewInt(1_000_000), big.NewInt(10_000), big.NewInt(1), "payroll-mainnet"),
		eventLog(t, contractpayroll.SigPayrollBatchExecuted, [][]byte{addressTopic(employer), addressTopic(token), addressTopic(token)}, big.NewInt(1_010_000), big.NewInt(1_000_000), big.NewInt(10_000), big.NewInt(1), "payroll-mainnet"),
	}
	return intent, plan, providers.Receipt{Status: providers.ReceiptSuccess, ChainID: contracts.ChainIDArcMainnet, TransactionHash: payrollTestHash, HasTransaction: true, From: employer, To: contracts.AddressWizPayPayroll, Value: "0x0", Input: plan.EncodedCall().CallData(), Logs: logs}
}

func eventLog(t *testing.T, signature string, indexed [][]byte, values ...any) providers.ReceiptLog {
	t.Helper()
	event, err := contractpayroll.EventBySignature(signature)
	if err != nil {
		t.Fatal(err)
	}
	data, err := event.Inputs.NonIndexed().Pack(values...)
	if err != nil {
		t.Fatal(err)
	}
	topics := [][]byte{event.ID.Bytes()}
	for _, topic := range indexed {
		topics = append(topics, common.LeftPadBytes(topic, 32))
	}
	return providers.ReceiptLog{Address: contracts.AddressWizPayPayroll, Topics: topics, Data: data}
}
func addressTopic(value string) []byte { return common.HexToAddress(value).Bytes() }

func TestMainnetPayrollVerifierValidSameToken(t *testing.T) {
	for _, tc := range []struct{ name, token, symbol string }{{"USDC", contracts.AddressUSDCMainnet, "USDC"}, {"EURC", contracts.AddressEURCMainnet, "EURC"}} {
		t.Run(tc.name, func(t *testing.T) {
			intent, plan, receipt := sameTokenFixture(t, tc.token, tc.symbol)
			result, err := NewVerifier(nil).Verify(intent, plan, receipt)
			if err != nil || !result.FinancialComplete() || result.Status != DomainVerified {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
	}
}

func TestMainnetPayrollVerifierRejectsCriticalMismatches(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*providers.Receipt)
	}{
		{"wrong sender", func(r *providers.Receipt) { r.From = "0x9999999999999999999999999999999999999999" }},
		{"wrong contract", func(r *providers.Receipt) { r.To = contracts.AddressUSDCMainnet }},
		{"native value", func(r *providers.Receipt) { r.Value = "0x1" }},
		{"wrong selector", func(r *providers.Receipt) { r.Input[0] ^= 1 }},
		{"missing payment", func(r *providers.Receipt) { r.Logs = r.Logs[1:] }},
		{"wrong payment", func(r *providers.Receipt) { r.Logs[0].Data[len(r.Logs[0].Data)-1] ^= 1 }},
		{"missing aggregate", func(r *providers.Receipt) { r.Logs = r.Logs[:2] }},
		{"failed receipt", func(r *providers.Receipt) { r.Status = providers.ReceiptReverted }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			intent, plan, receipt := sameTokenFixture(t, contracts.AddressUSDCMainnet, "USDC")
			receipt.Input = append([]byte(nil), receipt.Input...)
			receipt.Logs = providers.CloneLogs(receipt.Logs)
			tc.mutate(&receipt)
			result, err := NewVerifier(nil).Verify(intent, plan, receipt)
			if err != nil {
				t.Fatal(err)
			}
			if result.FinancialComplete() || !result.DefinitiveFailure() {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestMainnetPayrollVerifierPreservesBindings(t *testing.T) {
	intent, plan, receipt := sameTokenFixture(t, contracts.AddressUSDCMainnet, "USDC")
	if _, err := NewVerifier(nil).Verify(intent, Plan{}, providers.Receipt{}); err == nil {
		t.Fatal("empty plan must fail binding validation")
	}
	plan.intentID = "other-intent"
	if _, err := NewVerifier(nil).Verify(intent, plan, receipt); err == nil {
		t.Fatal("intent binding mismatch must be rejected")
	}
}
