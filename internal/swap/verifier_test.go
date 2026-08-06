package swap

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractswap "github.com/deseti/wizpay-mcp/internal/contracts/swap"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

const swapVerifierTxHash = "0x1111111111111111111111111111111111111111111111111111111111111111"

func TestSwapExecutedVerified(t *testing.T) {
	intent := swapIntent(t)
	plan, err := (Planner{}).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	financial := intent.Financial().Swap
	minOut := mustBase(t, financial.MinimumOutput)
	// amountOut exactly at minimum.
	receipt := successReceipt(t, swapExecutedLog(t,
		intent.Ownership().WalletAddress,
		financial.Router,
		financial.InputToken.Address,
		financial.OutputToken.Address,
		mustBase(t, financial.InputAmount),
		big.NewInt(1000), // fee
		new(big.Int).Sub(mustBase(t, financial.InputAmount), big.NewInt(1000)),
		minOut,
		financial.Recipient,
	))
	result, err := (Verifier{}).Verify(intent, plan, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != DomainVerified || !result.FinancialComplete() {
		t.Fatalf("result = %#v", result)
	}
}

func TestSwapExecutedAmountOutAboveMinimum(t *testing.T) {
	intent := swapIntent(t)
	plan, err := (Planner{}).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	financial := intent.Financial().Swap
	above := new(big.Int).Add(mustBase(t, financial.MinimumOutput), big.NewInt(1))
	receipt := successReceipt(t, swapExecutedLog(t,
		intent.Ownership().WalletAddress, financial.Router,
		financial.InputToken.Address, financial.OutputToken.Address,
		mustBase(t, financial.InputAmount), big.NewInt(0), mustBase(t, financial.InputAmount),
		above, financial.Recipient,
	))
	result, err := (Verifier{}).Verify(intent, plan, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != DomainVerified {
		t.Fatalf("result = %#v", result)
	}
}

func TestSwapExecutedFailures(t *testing.T) {
	intent := swapIntent(t)
	plan, err := (Planner{}).Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	financial := intent.Financial().Swap
	amountIn := mustBase(t, financial.InputAmount)
	minOut := mustBase(t, financial.MinimumOutput)
	goodOut := new(big.Int).Add(minOut, big.NewInt(100))
	base := func() providers.ReceiptLog {
		return swapExecutedLog(t,
			intent.Ownership().WalletAddress, financial.Router,
			financial.InputToken.Address, financial.OutputToken.Address,
			amountIn, big.NewInt(0), amountIn, goodOut, financial.Recipient,
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
				r.Logs[0].Address = contracts.AddressWizPayPayroll
				return r
			},
			status: DomainUnverified, code: "SWAP_EXECUTED_NOT_FOUND",
		},
		{
			name: "wrong user",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs = []providers.ReceiptLog{swapExecutedLog(t,
					"0x9999999999999999999999999999999999999999", financial.Router,
					financial.InputToken.Address, financial.OutputToken.Address,
					amountIn, big.NewInt(0), amountIn, goodOut, financial.Recipient,
				)}
				return r
			},
			status: DomainFailed, code: "SWAP_EXECUTED_USER_MISMATCH",
		},
		{
			name: "wrong router",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs = []providers.ReceiptLog{swapExecutedLog(t,
					intent.Ownership().WalletAddress, "0x9999999999999999999999999999999999999999",
					financial.InputToken.Address, financial.OutputToken.Address,
					amountIn, big.NewInt(0), amountIn, goodOut, financial.Recipient,
				)}
				return r
			},
			status: DomainFailed, code: "SWAP_EXECUTED_ROUTER_MISMATCH",
		},
		{
			name: "wrong tokenIn",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs = []providers.ReceiptLog{swapExecutedLog(t,
					intent.Ownership().WalletAddress, financial.Router,
					"0x9999999999999999999999999999999999999999", financial.OutputToken.Address,
					amountIn, big.NewInt(0), amountIn, goodOut, financial.Recipient,
				)}
				return r
			},
			status: DomainFailed, code: "SWAP_EXECUTED_TOKEN_IN_MISMATCH",
		},
		{
			name: "wrong tokenOut",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs = []providers.ReceiptLog{swapExecutedLog(t,
					intent.Ownership().WalletAddress, financial.Router,
					financial.InputToken.Address, "0x9999999999999999999999999999999999999999",
					amountIn, big.NewInt(0), amountIn, goodOut, financial.Recipient,
				)}
				return r
			},
			status: DomainFailed, code: "SWAP_EXECUTED_TOKEN_OUT_MISMATCH",
		},
		{
			name: "wrong amountIn",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs = []providers.ReceiptLog{swapExecutedLog(t,
					intent.Ownership().WalletAddress, financial.Router,
					financial.InputToken.Address, financial.OutputToken.Address,
					big.NewInt(1), big.NewInt(0), big.NewInt(1), goodOut, financial.Recipient,
				)}
				return r
			},
			status: DomainFailed, code: "SWAP_EXECUTED_AMOUNT_IN_MISMATCH",
		},
		{
			name: "amountOut below minimum",
			mutate: func(r providers.Receipt) providers.Receipt {
				below := new(big.Int).Sub(minOut, big.NewInt(1))
				r.Logs = []providers.ReceiptLog{swapExecutedLog(t,
					intent.Ownership().WalletAddress, financial.Router,
					financial.InputToken.Address, financial.OutputToken.Address,
					amountIn, big.NewInt(0), amountIn, below, financial.Recipient,
				)}
				return r
			},
			status: DomainFailed, code: "SWAP_EXECUTED_AMOUNT_OUT_BELOW_MINIMUM",
		},
		{
			name: "wrong recipient",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs = []providers.ReceiptLog{swapExecutedLog(t,
					intent.Ownership().WalletAddress, financial.Router,
					financial.InputToken.Address, financial.OutputToken.Address,
					amountIn, big.NewInt(0), amountIn, goodOut,
					"0x9999999999999999999999999999999999999999",
				)}
				return r
			},
			status: DomainFailed, code: "SWAP_EXECUTED_RECIPIENT_MISMATCH",
		},
		{
			name: "malformed event",
			mutate: func(r providers.Receipt) providers.Receipt {
				topic, _ := contractswap.EventTopic0()
				r.Logs = []providers.ReceiptLog{{
					Address: contracts.AddressWizPaySwapExecutor,
					Topics: [][]byte{
						topic,
						contracts.TopicAddress(intent.Ownership().WalletAddress),
						contracts.TopicAddress(financial.Router),
						contracts.TopicAddress(financial.InputToken.Address),
					},
					Data: []byte{0xde, 0xad},
				}}
				return r
			},
			status: DomainUnverified, code: "SWAP_EXECUTED_DECODE_ERROR",
		},
		{
			name: "duplicate conflicting events",
			mutate: func(r providers.Receipt) providers.Receipt {
				other := swapExecutedLog(t,
					intent.Ownership().WalletAddress, financial.Router,
					financial.InputToken.Address, financial.OutputToken.Address,
					amountIn, big.NewInt(0), amountIn, new(big.Int).Add(goodOut, big.NewInt(1)), financial.Recipient,
				)
				r.Logs = append(r.Logs, other)
				return r
			},
			status: DomainFailed, code: "SWAP_EXECUTED_AMBIGUOUS",
		},
		{
			name: "provider receipt success without valid event",
			mutate: func(r providers.Receipt) providers.Receipt {
				r.Logs = nil
				return r
			},
			status: DomainUnverified, code: "SWAP_EXECUTED_NOT_FOUND",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			receipt := successReceipt(t, base())
			receipt = tt.mutate(receipt)
			result, err := (Verifier{}).Verify(intent, plan, receipt)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != tt.status || result.ReasonCode != tt.code {
				t.Fatalf("got status=%s code=%s want %s/%s", result.Status, result.ReasonCode, tt.status, tt.code)
			}
			if result.FinancialComplete() {
				t.Fatal("must not be financially complete")
			}
		})
	}
}

func successReceipt(t *testing.T, logs ...providers.ReceiptLog) providers.Receipt {
	t.Helper()
	return providers.Receipt{
		Status:          providers.ReceiptSuccess,
		ChainID:         contracts.ChainIDArcTestnet,
		TransactionHash: swapVerifierTxHash,
		BlockNumber:     10,
		BlockHash:       "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Confirmations:   1,
		Logs:            logs,
	}
}

func swapExecutedLog(t *testing.T, user, router, tokenIn, tokenOut string, amountIn, fee, netIn, amountOut *big.Int, recipient string) providers.ReceiptLog {
	t.Helper()
	event, err := contractswap.EventBySignature(contractswap.SigWizPaySwapExecuted)
	if err != nil {
		t.Fatal(err)
	}
	data, err := event.Inputs.NonIndexed().Pack(
		common.HexToAddress(tokenOut),
		amountIn, fee, netIn, amountOut,
		common.HexToAddress(recipient),
	)
	if err != nil {
		t.Fatal(err)
	}
	return providers.ReceiptLog{
		Address: contracts.AddressWizPaySwapExecutor,
		Topics: [][]byte{
			event.ID.Bytes(),
			contracts.TopicAddress(user),
			contracts.TopicAddress(router),
			contracts.TopicAddress(tokenIn),
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
