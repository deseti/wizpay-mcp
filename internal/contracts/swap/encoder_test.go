package swap_test

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/contracts/swap"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func TestEncodeExecuteSwapDeterministic(t *testing.T) {
	registry := contracts.DefaultRegistry()
	in := swap.ExecuteSwapInput{
		Router:       "0x1111111111111111111111111111111111111111",
		TokenIn:      "0x3600000000000000000000000000000000000000",
		TokenOut:     "0x3600000000000000000000000000000000000001",
		AmountIn:     big.NewInt(1_000_000),
		MinAmountOut: big.NewInt(990_000),
		Recipient:    "0x2222222222222222222222222222222222222222",
		Deadline:     time.Now().Add(time.Hour).Unix(),
	}
	first, err := swap.EncodeExecuteSwap(registry, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := swap.EncodeExecuteSwap(registry, in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.CallData(), second.CallData()) {
		t.Fatal("encoding is not deterministic")
	}
	wantSel := contracts.Selector4(swap.SigExecuteSwap)
	if first.Selector() != wantSel {
		t.Fatalf("selector = %x, want %x", first.Selector(), wantSel)
	}
	sel := first.Selector()
	if hex.EncodeToString(sel[:]) != "88e290b2" {
		t.Fatalf("unexpected executeSwap selector %x", sel)
	}
	if !contracts.AddressesEqual(first.To(), contracts.AddressWizPaySwapExecutor) {
		t.Fatalf("To = %q", first.To())
	}
	if first.ChainID() != contracts.ChainIDArcTestnet {
		t.Fatalf("chain = %q", first.ChainID())
	}
}

func TestEncodeExecuteSwapDoesNotReadWallClock(t *testing.T) {
	call, err := swap.EncodeExecuteSwap(contracts.DefaultRegistry(), swap.ExecuteSwapInput{
		Router:       "0x1111111111111111111111111111111111111111",
		TokenIn:      "0x3600000000000000000000000000000000000000",
		TokenOut:     "0x3600000000000000000000000000000000000001",
		AmountIn:     big.NewInt(1_000_000),
		MinAmountOut: big.NewInt(990_000),
		Recipient:    "0x2222222222222222222222222222222222222222",
		Deadline:     1,
	})
	if err != nil {
		t.Fatalf("positive frozen deadline must encode without a wall-clock read: %v", err)
	}
	if call.Function() != swap.SigExecuteSwap {
		t.Fatalf("function = %q", call.Function())
	}
}

func TestSwapValidationRejections(t *testing.T) {
	registry := contracts.DefaultRegistry()
	base := swap.ExecuteSwapInput{
		Router:       "0x1111111111111111111111111111111111111111",
		TokenIn:      "0x3600000000000000000000000000000000000000",
		TokenOut:     "0x3600000000000000000000000000000000000001",
		AmountIn:     big.NewInt(1000),
		MinAmountOut: big.NewInt(900),
		Recipient:    "0x2222222222222222222222222222222222222222",
		Deadline:     time.Now().Add(time.Hour).Unix(),
	}
	cases := map[string]func(*swap.ExecuteSwapInput){
		"invalid router":     func(in *swap.ExecuteSwapInput) { in.Router = "bad" },
		"invalid tokenIn":    func(in *swap.ExecuteSwapInput) { in.TokenIn = "0xzz" },
		"invalid tokenOut":   func(in *swap.ExecuteSwapInput) { in.TokenOut = "" },
		"zero amountIn":      func(in *swap.ExecuteSwapInput) { in.AmountIn = big.NewInt(0) },
		"zero minAmountOut":  func(in *swap.ExecuteSwapInput) { in.MinAmountOut = big.NewInt(0) },
		"invalid recipient":  func(in *swap.ExecuteSwapInput) { in.Recipient = "0x0" },
		"zero deadline":      func(in *swap.ExecuteSwapInput) { in.Deadline = 0 },
		"zero address token": func(in *swap.ExecuteSwapInput) { in.TokenIn = "0x0000000000000000000000000000000000000000" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := base
			mutate(&in)
			if _, err := swap.EncodeExecuteSwap(registry, in); !hasCode(err, apperrors.CodeValidationError) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestSwapRegisteredAddressExact(t *testing.T) {
	call, err := swap.EncodeExecuteSwap(nil, swap.ExecuteSwapInput{
		Router:       "0x1111111111111111111111111111111111111111",
		TokenIn:      "0x3600000000000000000000000000000000000000",
		TokenOut:     "0x3600000000000000000000000000000000000001",
		AmountIn:     big.NewInt(1),
		MinAmountOut: big.NewInt(1),
		Recipient:    "0x2222222222222222222222222222222222222222",
		Deadline:     time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !contracts.AddressesEqual(call.To(), contracts.AddressWizPaySwapExecutor) {
		t.Fatalf("To = %q", call.To())
	}
}

func hasCode(err error, code apperrors.Code) bool {
	if err == nil {
		return false
	}
	return apperrors.ToPublic(err).Code == code
}
