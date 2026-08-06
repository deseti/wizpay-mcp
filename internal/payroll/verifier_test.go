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

const verifierTxHash = "0x1111111111111111111111111111111111111111111111111111111111111111"

func TestSinglePaymentRoutedVerified(t *testing.T) {
	intent := payrollIntent(t, intents.PayrollVariantSingle)
	plan, err := (Planner{}).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	financial := intent.Financial().Payroll
	line := financial.Recipients[0]
	receipt := successReceipt(t, paymentRoutedLog(t,
		intent.Ownership().WalletAddress,
		line.Address,
		financial.TokenIn.Address,
		line.TokenOut.Address,
		mustBase(t, line.AmountIn),
		mustBase(t, line.MinAmountOut), // exact minimum still passes
		big.NewInt(5),
	))

	result, err := (Verifier{}).Verify(intent, plan, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != DomainVerified || !result.FinancialComplete() {
		t.Fatalf("result = %#v", result)
	}
	if result.EventSignature != contractpayroll.SigPaymentRouted {
		t.Fatalf("event = %q", result.EventSignature)
	}
}

func TestSinglePaymentRoutedFailures(t *testing.T) {
	intent := payrollIntent(t, intents.PayrollVariantSingle)
	plan, err := (Planner{}).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	financial := intent.Financial().Payroll
	line := financial.Recipients[0]
	baseLog := func() providers.ReceiptLog {
		return paymentRoutedLog(t,
			intent.Ownership().WalletAddress, line.Address,
			financial.TokenIn.Address, line.TokenOut.Address,
			mustBase(t, line.AmountIn), big.NewInt(2_000_000), big.NewInt(5),
		)
	}

	cases := []struct {
		name   string
		mutate func(providers.Receipt) providers.Receipt
		status DomainStatus
		code   string
	}{
		{
			name: "wrong contract",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs[0].Address = contracts.AddressWizPaySwapExecutor
				return r
			},
			status: DomainUnverified,
			code:   "PAYMENT_ROUTED_NOT_FOUND",
		},
		{
			name: "wrong event signature",
			mutate: func(r providers.Receipt) providers.Receipt {
				topic, _ := contractpayroll.EventTopic0(contractpayroll.SigBatchPaymentRouted)
				r.Logs[0].Topics[0] = topic
				return r
			},
			status: DomainUnverified,
			code:   "PAYMENT_ROUTED_NOT_FOUND",
		},
		{
			name: "wrong recipient",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs = []providers.ReceiptLog{paymentRoutedLog(t,
					intent.Ownership().WalletAddress, "0x9999999999999999999999999999999999999999",
					financial.TokenIn.Address, line.TokenOut.Address,
					mustBase(t, line.AmountIn), big.NewInt(2_000_000), big.NewInt(5),
				)}
				return r
			},
			status: DomainFailed,
			code:   "PAYMENT_ROUTED_RECIPIENT_MISMATCH",
		},
		{
			name: "wrong tokenIn",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs = []providers.ReceiptLog{paymentRoutedLog(t,
					intent.Ownership().WalletAddress, line.Address,
					"0x9999999999999999999999999999999999999999", line.TokenOut.Address,
					mustBase(t, line.AmountIn), big.NewInt(2_000_000), big.NewInt(5),
				)}
				return r
			},
			status: DomainFailed,
			code:   "PAYMENT_ROUTED_TOKEN_IN_MISMATCH",
		},
		{
			name: "wrong tokenOut",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs = []providers.ReceiptLog{paymentRoutedLog(t,
					intent.Ownership().WalletAddress, line.Address,
					financial.TokenIn.Address, "0x9999999999999999999999999999999999999999",
					mustBase(t, line.AmountIn), big.NewInt(2_000_000), big.NewInt(5),
				)}
				return r
			},
			status: DomainFailed,
			code:   "PAYMENT_ROUTED_TOKEN_OUT_MISMATCH",
		},
		{
			name: "wrong amountIn",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs = []providers.ReceiptLog{paymentRoutedLog(t,
					intent.Ownership().WalletAddress, line.Address,
					financial.TokenIn.Address, line.TokenOut.Address,
					big.NewInt(1), big.NewInt(2_000_000), big.NewInt(5),
				)}
				return r
			},
			status: DomainFailed,
			code:   "PAYMENT_ROUTED_AMOUNT_IN_MISMATCH",
		},
		{
			name: "malformed event",
			mutate: func(r providers.Receipt) providers.Receipt {
				topic, _ := contractpayroll.EventTopic0(contractpayroll.SigPaymentRouted)
				r.Logs = []providers.ReceiptLog{{
					Address: contracts.AddressWizPayPayroll,
					Topics: [][]byte{
						topic,
						contracts.TopicAddress(intent.Ownership().WalletAddress),
						contracts.TopicAddress(line.Address),
					},
					Data: []byte{0x01, 0x02},
				}}
				return r
			},
			status: DomainUnverified,
			code:   "PAYMENT_ROUTED_DECODE_ERROR",
		},
		{
			name: "zero matching events",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs = nil
				return r
			},
			status: DomainUnverified,
			code:   "PAYMENT_ROUTED_NOT_FOUND",
		},
		{
			name: "conflicting multiple events",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs = []providers.ReceiptLog{
					baseLog(),
					paymentRoutedLog(t,
						intent.Ownership().WalletAddress, line.Address,
						financial.TokenIn.Address, line.TokenOut.Address,
						mustBase(t, line.AmountIn), big.NewInt(3_000_000), big.NewInt(5),
					),
				}
				return r
			},
			status: DomainFailed,
			code:   "PAYMENT_ROUTED_AMBIGUOUS",
		},
		{
			name: "receipt not success",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Status = providers.ReceiptUnknown
				return r
			},
			status: DomainUnverified,
			code:   "RECEIPT_NOT_SUCCESS",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			receipt := successReceipt(t, baseLog())
			receipt = tt.mutate(receipt)
			result, err := (Verifier{}).Verify(intent, plan, receipt)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != tt.status || result.ReasonCode != tt.code {
				t.Fatalf("got status=%s code=%s want %s/%s", result.Status, result.ReasonCode, tt.status, tt.code)
			}
			if result.FinancialComplete() {
				t.Fatal("failure path must not be financially complete")
			}
		})
	}
}

func TestBatchPaymentRoutedAggregateOnly(t *testing.T) {
	for _, variant := range []intents.PayrollVariant{
		intents.PayrollVariantBatchSingleTokenOut,
		intents.PayrollVariantBatchMultiTokenOut,
	} {
		t.Run(string(variant), func(t *testing.T) {
			intent := payrollIntent(t, variant)
			plan, err := (Planner{}).Plan(intent)
			if err != nil {
				t.Fatal(err)
			}
			financial := intent.Financial().Payroll
			tokenOut := financial.Recipients[0].TokenOut.Address
			if variant == intents.PayrollVariantBatchMultiTokenOut {
				// Event has a single tokenOut field; use first line's token for packing.
				tokenOut = financial.Recipients[0].TokenOut.Address
			}
			minSum := new(big.Int)
			for _, line := range financial.Recipients {
				minSum.Add(minSum, mustBase(t, line.MinAmountOut))
			}
			// Total out exactly at sum of mins.
			receipt := successReceipt(t, batchPaymentRoutedLog(t,
				intent.Ownership().WalletAddress,
				financial.TokenIn.Address,
				tokenOut,
				mustBase(t, financial.Total),
				minSum,
				big.NewInt(10),
				big.NewInt(int64(len(financial.Recipients))),
				financial.ReferenceID,
			))
			result, err := (Verifier{}).Verify(intent, plan, receipt)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != DomainAggregateOnly {
				t.Fatalf("status = %s (%s)", result.Status, result.ReasonCode)
			}
			if result.FinancialComplete() {
				t.Fatal("batch must never claim full financial completion")
			}
			if result.ReasonCode != "BATCH_RECIPIENT_LEVEL_UNPROVEN" {
				t.Fatalf("reason = %s", result.ReasonCode)
			}
			if len(result.Unprovable) == 0 {
				t.Fatal("must document recipient-level unprovable fields")
			}
			found := false
			for _, field := range result.Unprovable {
				if field == "per_recipient_address" {
					found = true
				}
			}
			if !found {
				t.Fatalf("unprovable = %v", result.Unprovable)
			}
		})
	}
}

func TestBatchPaymentRoutedFailures(t *testing.T) {
	intent := payrollIntent(t, intents.PayrollVariantBatchSingleTokenOut)
	plan, err := (Planner{}).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	financial := intent.Financial().Payroll
	tokenOut := financial.Recipients[0].TokenOut.Address
	minSum := new(big.Int)
	for _, line := range financial.Recipients {
		minSum.Add(minSum, mustBase(t, line.MinAmountOut))
	}
	good := batchPaymentRoutedLog(t,
		intent.Ownership().WalletAddress, financial.TokenIn.Address, tokenOut,
		mustBase(t, financial.Total), minSum, big.NewInt(10),
		big.NewInt(int64(len(financial.Recipients))), financial.ReferenceID,
	)

	t.Run("wrong reference id", func(t *testing.T) {
		log := batchPaymentRoutedLog(t,
			intent.Ownership().WalletAddress, financial.TokenIn.Address, tokenOut,
			mustBase(t, financial.Total), minSum, big.NewInt(10),
			big.NewInt(int64(len(financial.Recipients))), "wrong-ref",
		)
		result, err := (Verifier{}).Verify(intent, plan, successReceipt(t, log))
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != DomainFailed || result.ReasonCode != "BATCH_PAYMENT_ROUTED_REFERENCE_MISMATCH" {
			t.Fatalf("got %#v", result)
		}
	})
	t.Run("wrong aggregate amount", func(t *testing.T) {
		log := batchPaymentRoutedLog(t,
			intent.Ownership().WalletAddress, financial.TokenIn.Address, tokenOut,
			big.NewInt(1), minSum, big.NewInt(10),
			big.NewInt(int64(len(financial.Recipients))), financial.ReferenceID,
		)
		result, err := (Verifier{}).Verify(intent, plan, successReceipt(t, log))
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != DomainFailed || result.ReasonCode != "BATCH_PAYMENT_ROUTED_TOTAL_IN_MISMATCH" {
			t.Fatalf("got %#v", result)
		}
	})
	t.Run("wrong token out", func(t *testing.T) {
		log := batchPaymentRoutedLog(t,
			intent.Ownership().WalletAddress, financial.TokenIn.Address,
			"0x9999999999999999999999999999999999999999",
			mustBase(t, financial.Total), minSum, big.NewInt(10),
			big.NewInt(int64(len(financial.Recipients))), financial.ReferenceID,
		)
		result, err := (Verifier{}).Verify(intent, plan, successReceipt(t, log))
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != DomainFailed || result.ReasonCode != "BATCH_PAYMENT_ROUTED_TOKEN_OUT_MISMATCH" {
			t.Fatalf("got %#v", result)
		}
	})
	t.Run("duplicate conflicting batch events", func(t *testing.T) {
		other := batchPaymentRoutedLog(t,
			intent.Ownership().WalletAddress, financial.TokenIn.Address, tokenOut,
			mustBase(t, financial.Total), new(big.Int).Add(minSum, big.NewInt(1)), big.NewInt(10),
			big.NewInt(int64(len(financial.Recipients))), financial.ReferenceID,
		)
		receipt := successReceipt(t, good)
		receipt.Logs = append(receipt.Logs, other)
		result, err := (Verifier{}).Verify(intent, plan, receipt)
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != DomainFailed || result.ReasonCode != "BATCH_PAYMENT_ROUTED_AMBIGUOUS" {
			t.Fatalf("got %#v", result)
		}
	})
}

func TestBatchDoesNotOverclaimRecipientSettlement(t *testing.T) {
	// Explicit documentation test: even a perfect aggregate match never yields
	// DomainVerified / FinancialComplete.
	intent := payrollIntent(t, intents.PayrollVariantBatchSingleTokenOut)
	plan, err := (Planner{}).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	financial := intent.Financial().Payroll
	minSum := new(big.Int)
	for _, line := range financial.Recipients {
		minSum.Add(minSum, mustBase(t, line.MinAmountOut))
	}
	receipt := successReceipt(t, batchPaymentRoutedLog(t,
		intent.Ownership().WalletAddress, financial.TokenIn.Address,
		financial.Recipients[0].TokenOut.Address,
		mustBase(t, financial.Total), minSum, big.NewInt(0),
		big.NewInt(int64(len(financial.Recipients))), financial.ReferenceID,
	))
	result, err := (Verifier{}).Verify(intent, plan, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status == DomainVerified || result.FinancialComplete() {
		t.Fatalf("overclaim: %#v", result)
	}
	if result.Status != DomainAggregateOnly {
		t.Fatalf("expected aggregate-only, got %#v", result)
	}
}

func successReceipt(t *testing.T, logs ...providers.ReceiptLog) providers.Receipt {
	t.Helper()
	return providers.Receipt{
		Status:          providers.ReceiptSuccess,
		ChainID:         contracts.ChainIDArcTestnet,
		TransactionHash: verifierTxHash,
		BlockNumber:     10,
		BlockHash:       "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Confirmations:   1,
		Logs:            logs,
	}
}

func paymentRoutedLog(t *testing.T, sender, recipient, tokenIn, tokenOut string, amountIn, amountOut, fee *big.Int) providers.ReceiptLog {
	t.Helper()
	event, err := contractpayroll.EventBySignature(contractpayroll.SigPaymentRouted)
	if err != nil {
		t.Fatal(err)
	}
	data, err := event.Inputs.NonIndexed().Pack(
		common.HexToAddress(tokenIn),
		common.HexToAddress(tokenOut),
		amountIn, amountOut, fee,
	)
	if err != nil {
		t.Fatal(err)
	}
	return providers.ReceiptLog{
		Address: contracts.AddressWizPayPayroll,
		Topics: [][]byte{
			event.ID.Bytes(),
			contracts.TopicAddress(sender),
			contracts.TopicAddress(recipient),
		},
		Data: data,
	}
}

func batchPaymentRoutedLog(t *testing.T, sender, tokenIn, tokenOut string, totalIn, totalOut, fees, count *big.Int, ref string) providers.ReceiptLog {
	t.Helper()
	event, err := contractpayroll.EventBySignature(contractpayroll.SigBatchPaymentRouted)
	if err != nil {
		t.Fatal(err)
	}
	data, err := event.Inputs.NonIndexed().Pack(
		common.HexToAddress(tokenIn),
		common.HexToAddress(tokenOut),
		totalIn, totalOut, fees, count, ref,
	)
	if err != nil {
		t.Fatal(err)
	}
	return providers.ReceiptLog{
		Address: contracts.AddressWizPayPayroll,
		Topics: [][]byte{
			event.ID.Bytes(),
			contracts.TopicAddress(sender),
		},
		Data: data,
	}
}

func mustBase(t *testing.T, amount intents.Amount) *big.Int {
	t.Helper()
	value, err := amount.BaseInt()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
