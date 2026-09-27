package payroll

import (
	"bytes"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractpayroll "github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

const crossHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func crossIntent(t *testing.T, tokenIn string) intents.Intent {
	t.Helper()
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	inSymbol, outAddress, outSymbol := "USDC", contracts.AddressEURCMainnet, "EURC"
	if contracts.AddressesEqual(tokenIn, contracts.AddressEURCMainnet) {
		inSymbol, outAddress, outSymbol = "EURC", contracts.AddressUSDCMainnet, "USDC"
	}
	in := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: tokenIn, Symbol: inSymbol, Decimals: 6}
	out := intents.Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: outAddress, Symbol: outSymbol, Decimals: 6}
	amount := intents.Amount{Decimal: "5", BaseUnits: "5000000", Decimals: 6}
	owner := intents.Ownership{UserID: "user", IdentityProvider: "circle", ProviderUserReference: "provider-user", WalletBindingID: "binding", WalletBindingVersion: 1, WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet}
	value, err := intents.NewDraft(intents.Params{IntentID: "cross", Version: 1, ClientRequestID: "request", Nonce: "nonce", Type: intents.TypePayroll, Ownership: owner, Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{SchemaVersion: intents.FinancialSchemaPhase12, Variant: intents.PayrollVariantBatchSingleTokenOut, TokenIn: in, Recipients: []intents.Recipient{{Address: "0x3333333333333333333333333333333333333333", TokenOut: out, MinAmountOut: amount}, {Address: "0x4444444444444444444444444444444444444444", TokenOut: out, MinAmountOut: amount}}, Total: intents.Amount{Decimal: "10", BaseUnits: "10000000", Decimals: 6}, ReferenceID: "cross-reference", CrossToken: &intents.CrossTokenPayrollParameters{GrossInput: intents.Amount{Decimal: "11", BaseUnits: "11000000", Decimals: 6}, MinTotalOut: intents.Amount{Decimal: "10", BaseUnits: "10000000", Decimals: 6}, MinHopPriceX36: "1", Deadline: now.Add(10 * time.Minute)}}}, Route: intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferencePayroll, Version: intents.RouteVersionPayroll}, Constraints: intents.Constraints{Deadline: now.Add(20 * time.Minute), PolicyReference: "policy:1"}, CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	value, err = value.Transition(intents.StatusCreated, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestCrossTokenPlannerSealsBothDirections(t *testing.T) {
	for _, input := range []string{contracts.AddressUSDCMainnet, contracts.AddressEURCMainnet} {
		t.Run(input, func(t *testing.T) {
			intent := crossIntent(t, input)
			plan, err := NewPlanner(nil).Plan(intent)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := contractpayroll.DecodeCrossTokenPayrollCall(plan.EncodedCall().CallData())
			if err != nil {
				t.Fatal(err)
			}
			if plan.EncodedCall().To() != contracts.AddressWizPayPayroll || decoded.GrossInput.String() != "11000000" || decoded.MinTotalOut.String() != "10000000" || decoded.MinHopPriceX36.String() != "1" || decoded.ReferenceID != "cross-reference" || len(decoded.Recipients) != 2 || decoded.OutputAmounts[0].String() != "5000000" {
				t.Fatalf("decoded=%#v", decoded)
			}
			again, err := NewPlanner(nil).Plan(intent)
			if err != nil || !bytes.Equal(plan.EncodedCall().CallData(), again.EncodedCall().CallData()) {
				t.Fatal("cross-token plan is not deterministic")
			}
		})
	}
}

func crossReceipt(t *testing.T, intent intents.Intent, plan Plan, amountOut *big.Int) providers.Receipt {
	t.Helper()
	f := intent.Financial().Payroll
	gross, _ := f.CrossToken.GrossInput.BaseInt()
	minimum, _ := f.CrossToken.MinTotalOut.BaseInt()
	fee := big.NewInt(25_000)
	net := new(big.Int).Sub(new(big.Int).Set(gross), fee)
	obligations := big.NewInt(10_000_000)
	refHash, _ := contractpayroll.ReferenceHash(intent.Ownership().WalletAddress, f.ReferenceID)
	tokenOut := f.Recipients[0].TokenOut.Address
	value := new(big.Int)
	if contracts.AddressesEqual(f.TokenIn.Address, contracts.AddressUSDCMainnet) {
		value.Mul(gross, big.NewInt(1_000_000_000_000))
	}
	r := providers.Receipt{Status: providers.ReceiptSuccess, ChainID: contracts.ChainIDArcMainnet, TransactionHash: crossHash, HasTransaction: true, From: intent.Ownership().WalletAddress, To: contracts.AddressWizPayPayroll, Value: hexutil.EncodeBig(value), Input: plan.EncodedCall().CallData()}
	r.Logs = append(r.Logs, payrollEvent(t, contractpayroll.SigPayrollSwapExecuted, [][]byte{refHash[:], contracts.TopicAddress(r.From), contracts.TopicAddress(f.TokenIn.Address)}, common.HexToAddress(tokenOut), gross, fee, net, amountOut, minimum))
	for i, line := range f.Recipients {
		amount, _ := line.MinAmountOut.BaseInt()
		r.Logs = append(r.Logs, payrollEvent(t, contractpayroll.SigPayrollPayment, [][]byte{refHash[:], contracts.TopicAddress(r.From), contracts.TopicAddress(tokenOut)}, common.HexToAddress(line.Address), big.NewInt(int64(i)), amount))
	}
	var digest [32]byte
	r.Logs = append(r.Logs, payrollEvent(t, contractpayroll.SigPayrollReferenceConsumed, [][]byte{refHash[:], contracts.TopicAddress(r.From), contracts.TopicAddress(f.TokenIn.Address)}, common.HexToAddress(tokenOut), digest, gross, obligations, fee, big.NewInt(2), f.ReferenceID))
	r.Logs = append(r.Logs, payrollEvent(t, contractpayroll.SigPayrollBatchExecuted, [][]byte{contracts.TopicAddress(r.From), contracts.TopicAddress(f.TokenIn.Address), contracts.TopicAddress(tokenOut)}, gross, obligations, fee, big.NewInt(2), f.ReferenceID))
	surplus := new(big.Int).Sub(new(big.Int).Set(amountOut), obligations)
	if surplus.Sign() > 0 {
		r.Logs = append(r.Logs, payrollEvent(t, contractpayroll.SigPayrollSurplusRefunded, [][]byte{refHash[:], contracts.TopicAddress(r.From), contracts.TopicAddress(tokenOut)}, surplus))
	}
	return r
}

func payrollEvent(t *testing.T, signature string, indexed [][]byte, values ...any) providers.ReceiptLog {
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
	topics = append(topics, indexed...)
	return providers.ReceiptLog{Address: contracts.AddressWizPayPayroll, Topics: topics, Data: data}
}

func TestCrossTokenVerifierAcceptsBothDirectionsAndSurplusModes(t *testing.T) {
	for _, input := range []string{contracts.AddressUSDCMainnet, contracts.AddressEURCMainnet} {
		for _, out := range []*big.Int{big.NewInt(10_000_000), big.NewInt(10_500_000)} {
			intent := crossIntent(t, input)
			plan, _ := NewPlanner(nil).Plan(intent)
			result, err := NewVerifier(nil).Verify(intent, plan, crossReceipt(t, intent, plan, out))
			if err != nil || !result.FinancialComplete() {
				t.Fatalf("input=%s out=%s result=%#v err=%v", input, out, result, err)
			}
		}
	}
}

func TestCrossTokenVerifierRejectsFundLossContradictions(t *testing.T) {
	intent := crossIntent(t, contracts.AddressEURCMainnet)
	cases := []struct {
		name   string
		mutate func(*providers.Receipt)
	}{{"wrong_chain", func(r *providers.Receipt) { r.ChainID = "5042002" }}, {"revert", func(r *providers.Receipt) { r.Status = providers.ReceiptReverted }}, {"wrong_sender", func(r *providers.Receipt) { r.From = "0x9999999999999999999999999999999999999999" }}, {"wrong_target", func(r *providers.Receipt) { r.To = contracts.AddressWizPaySwapExecutor }}, {"wrong_value", func(r *providers.Receipt) { r.Value = "0x1" }}, {"missing_swap", func(r *providers.Receipt) { r.Logs = r.Logs[1:] }}, {"missing_payment", func(r *providers.Receipt) { r.Logs = append(r.Logs[:1], r.Logs[2:]...) }}, {"missing_reference", func(r *providers.Receipt) { r.Logs = append(r.Logs[:3], r.Logs[4:]...) }}, {"missing_batch", func(r *providers.Receipt) { r.Logs = append(r.Logs[:4], r.Logs[5:]...) }}, {"missing_surplus", func(r *providers.Receipt) { r.Logs = r.Logs[:len(r.Logs)-1] }}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, _ := NewPlanner(nil).Plan(intent)
			receipt := crossReceipt(t, intent, plan, big.NewInt(10_500_000))
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
