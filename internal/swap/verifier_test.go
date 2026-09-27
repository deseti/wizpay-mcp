package swap

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractswap "github.com/deseti/wizpay-mcp/internal/contracts/swap"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

const swapTestHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func swapFixture(t *testing.T, input string) (Plan, providers.Receipt) {
	t.Helper()
	intent := mainnetSwapIntentDirection(t, input)
	plan, err := NewPlanner(nil).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	amountIn, _ := intent.Financial().Swap.InputAmount.BaseInt()
	minimum, _ := intent.Financial().Swap.MinimumOutput.BaseInt()
	fee := big.NewInt(25_000)
	net := new(big.Int).Sub(new(big.Int).Set(amountIn), fee)
	output := big.NewInt(9_100_000)
	value := new(big.Int)
	if contracts.AddressesEqual(input, contracts.AddressUSDCMainnet) {
		value.Mul(amountIn, big.NewInt(1_000_000_000_000))
	}
	receipt := providers.Receipt{Status: providers.ReceiptSuccess, ChainID: contracts.ChainIDArcMainnet, TransactionHash: swapTestHash, HasTransaction: true, From: intent.Ownership().WalletAddress, To: contracts.AddressWizPaySwapExecutor, Value: hexutil.EncodeBig(value), Input: plan.EncodedCall().CallData()}
	receipt.Logs = []providers.ReceiptLog{
		swapEventLog(t, receipt.From, intent.Financial().Swap.InputToken.Address, intent.Financial().Swap.OutputToken.Address, amountIn, fee, net, output, minimum),
		transferLog(intent.Financial().Swap.OutputToken.Address, contracts.AddressWizPaySwapExecutor, receipt.From, output),
	}
	return plan, receipt
}

func swapEventLog(t *testing.T, caller, tokenIn, tokenOut string, amountIn, fee, net, amountOut, minimum *big.Int) providers.ReceiptLog {
	t.Helper()
	event, err := contractswap.EventBySignature(contractswap.SigWizPayMainnetSwapExecuted)
	if err != nil {
		t.Fatal(err)
	}
	data, err := event.Inputs.NonIndexed().Pack(amountIn, fee, net, amountOut, minimum)
	if err != nil {
		t.Fatal(err)
	}
	return providers.ReceiptLog{Address: contracts.AddressWizPaySwapExecutor, Topics: [][]byte{event.ID.Bytes(), contracts.TopicAddress(caller), contracts.TopicAddress(tokenIn), contracts.TopicAddress(tokenOut)}, Data: data}
}

func transferLog(token, from, to string, amount *big.Int) providers.ReceiptLog {
	return providers.ReceiptLog{Address: token, Topics: [][]byte{crypto.Keccak256([]byte("Transfer(address,address,uint256)")), contracts.TopicAddress(from), contracts.TopicAddress(to)}, Data: common.LeftPadBytes(amount.Bytes(), 32)}
}

func TestMainnetSwapVerifierAcceptsBothCanonicalDirections(t *testing.T) {
	for _, input := range []string{contracts.AddressUSDCMainnet, contracts.AddressEURCMainnet} {
		t.Run(input, func(t *testing.T) {
			intent := mainnetSwapIntentDirection(t, input)
			plan, receipt := swapFixture(t, input)
			result, err := NewVerifier(nil).Verify(intent, plan, receipt)
			if err != nil || !result.FinancialComplete() || result.DefinitiveFailure() {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
	}
}

func TestMainnetSwapVerifierRejectsTransactionContradictions(t *testing.T) {
	intent := mainnetSwapIntentDirection(t, contracts.AddressEURCMainnet)
	cases := []struct {
		name   string
		mutate func(*providers.Receipt)
	}{
		{"wrong_chain", func(r *providers.Receipt) { r.ChainID = "5042002" }},
		{"reverted", func(r *providers.Receipt) { r.Status = providers.ReceiptReverted }},
		{"wrong_sender", func(r *providers.Receipt) { r.From = "0x9999999999999999999999999999999999999999" }},
		{"wrong_target", func(r *providers.Receipt) { r.To = contracts.AddressWizPayPayroll }},
		{"wrong_selector", func(r *providers.Receipt) { r.Input[0] ^= 0xff }},
		{"wrong_value", func(r *providers.Receipt) { r.Value = "0x1" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, receipt := swapFixture(t, contracts.AddressEURCMainnet)
			receipt.Input = append([]byte(nil), receipt.Input...)
			tc.mutate(&receipt)
			result, err := NewVerifier(nil).Verify(intent, plan, receipt)
			if err != nil {
				t.Fatal(err)
			}
			if result.FinancialComplete() || !result.DefinitiveFailure() {
				t.Fatalf("result=%#v", result)
			}
		})
	}
}

func TestMainnetSwapVerifierRejectsEventAndOutputContradictions(t *testing.T) {
	intent := mainnetSwapIntentDirection(t, contracts.AddressEURCMainnet)
	cases := []struct {
		name   string
		mutate func(*providers.Receipt)
	}{
		{"missing_event", func(r *providers.Receipt) { r.Logs = r.Logs[1:] }},
		{"wrong_caller", func(r *providers.Receipt) {
			r.Logs[0].Topics[1] = contracts.TopicAddress("0x9999999999999999999999999999999999999999")
		}},
		{"wrong_token", func(r *providers.Receipt) { r.Logs[0].Topics[2] = contracts.TopicAddress(contracts.AddressUSDCMainnet) }},
		{"wrong_amount", func(r *providers.Receipt) {
			r.Logs[0] = swapEventLog(t, r.From, contracts.AddressEURCMainnet, contracts.AddressUSDCMainnet, big.NewInt(10_000_001), big.NewInt(25_000), big.NewInt(9_975_001), big.NewInt(9_100_000), big.NewInt(9_000_000))
		}},
		{"wrong_minimum", func(r *providers.Receipt) {
			r.Logs[0] = swapEventLog(t, r.From, contracts.AddressEURCMainnet, contracts.AddressUSDCMainnet, big.NewInt(10_000_000), big.NewInt(25_000), big.NewInt(9_975_000), big.NewInt(9_100_000), big.NewInt(9_000_001))
		}},
		{"below_minimum", func(r *providers.Receipt) {
			r.Logs[0] = swapEventLog(t, r.From, contracts.AddressEURCMainnet, contracts.AddressUSDCMainnet, big.NewInt(10_000_000), big.NewInt(25_000), big.NewInt(9_975_000), big.NewInt(8_999_999), big.NewInt(9_000_000))
		}},
		{"fee_arithmetic", func(r *providers.Receipt) {
			r.Logs[0] = swapEventLog(t, r.From, contracts.AddressEURCMainnet, contracts.AddressUSDCMainnet, big.NewInt(10_000_000), big.NewInt(25_000), big.NewInt(9_974_999), big.NewInt(9_100_000), big.NewInt(9_000_000))
		}},
		{"excessive_fee", func(r *providers.Receipt) {
			r.Logs[0] = swapEventLog(t, r.From, contracts.AddressEURCMainnet, contracts.AddressUSDCMainnet, big.NewInt(10_000_000), big.NewInt(100_001), big.NewInt(9_899_999), big.NewInt(9_100_000), big.NewInt(9_000_000))
		}},
		{"missing_output_transfer", func(r *providers.Receipt) { r.Logs = r.Logs[:1] }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, receipt := swapFixture(t, contracts.AddressEURCMainnet)
			receipt.Logs = providers.CloneLogs(receipt.Logs)
			tc.mutate(&receipt)
			result, err := NewVerifier(nil).Verify(intent, plan, receipt)
			if err != nil {
				t.Fatal(err)
			}
			if result.FinancialComplete() || !result.DefinitiveFailure() {
				t.Fatalf("result=%#v", result)
			}
		})
	}
}

func TestMainnetSwapVerifierPreservesPlanBindings(t *testing.T) {
	intent := mainnetSwapIntent(t)
	if _, err := NewVerifier(nil).Verify(intent, Plan{}, providers.Receipt{}); err == nil {
		t.Fatal("empty plan must fail")
	}
	plan, _ := swapFixture(t, contracts.AddressEURCMainnet)
	plan.intentID = "other"
	if _, err := NewVerifier(nil).Verify(intent, plan, providers.Receipt{}); err == nil {
		t.Fatal("mismatched plan must fail")
	}
}
