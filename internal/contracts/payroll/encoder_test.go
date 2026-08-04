package payroll_test

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func TestEncodeBatchMultiTokenOutDeterministic(t *testing.T) {
	registry := contracts.DefaultRegistry()
	in := payroll.BatchMultiTokenOut{
		TokenIn:       "0x3600000000000000000000000000000000000000",
		TokenOuts:     []string{"0x3600000000000000000000000000000000000000", "0x3600000000000000000000000000000000000001"},
		Recipients:    []string{"0x1111111111111111111111111111111111111111", "0x2222222222222222222222222222222222222222"},
		AmountsIn:     []*big.Int{big.NewInt(1000), big.NewInt(2000)},
		MinAmountsOut: []*big.Int{big.NewInt(900), big.NewInt(1800)},
		ReferenceID:   "payroll-batch-001",
	}
	first, err := payroll.EncodeBatchMultiTokenOut(registry, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := payroll.EncodeBatchMultiTokenOut(registry, in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.CallData(), second.CallData()) {
		t.Fatal("encoding is not deterministic")
	}
	wantSel := contracts.Selector4(payroll.SigBatchMultiTokenOut)
	if first.Selector() != wantSel {
		t.Fatalf("selector = %x, want %x", first.Selector(), wantSel)
	}
	if !bytes.Equal(first.CallData()[:4], wantSel[:]) {
		t.Fatal("calldata selector prefix mismatch")
	}
	if !contracts.AddressesEqual(first.To(), contracts.AddressWizPayPayroll) {
		t.Fatalf("To = %q", first.To())
	}
	if first.ChainID() != contracts.ChainIDArcTestnet {
		t.Fatalf("chain = %q", first.ChainID())
	}
	if first.Function() != payroll.SigBatchMultiTokenOut {
		t.Fatalf("function = %q", first.Function())
	}
	// CallData getter must return a defensive copy.
	mutated := first.CallData()
	mutated[0] ^= 0xff
	if bytes.Equal(first.CallData(), mutated) {
		t.Fatal("CallData getter did not return a defensive copy")
	}
}

func TestEncodeBatchSingleTokenOutDeterministic(t *testing.T) {
	registry := contracts.DefaultRegistry()
	in := payroll.BatchSingleTokenOut{
		TokenIn:       "0x3600000000000000000000000000000000000000",
		TokenOut:      "0x3600000000000000000000000000000000000000",
		Recipients:    []string{"0x1111111111111111111111111111111111111111"},
		AmountsIn:     []*big.Int{big.NewInt(5000)},
		MinAmountsOut: []*big.Int{big.NewInt(4900)},
		ReferenceID:   "payroll-single-token-batch",
	}
	call, err := payroll.EncodeBatchSingleTokenOut(registry, in)
	if err != nil {
		t.Fatal(err)
	}
	wantSel := contracts.Selector4(payroll.SigBatchSingleTokenOut)
	if call.Selector() != wantSel {
		t.Fatalf("selector = %x, want %x", call.Selector(), wantSel)
	}
	// Overloads must produce distinct selectors.
	multiSel := contracts.Selector4(payroll.SigBatchMultiTokenOut)
	if call.Selector() == multiSel {
		t.Fatal("single and multi overloads must not share a selector")
	}
}

func TestEncodeRouteAndPayDeterministic(t *testing.T) {
	registry := contracts.DefaultRegistry()
	in := payroll.SinglePayment{
		TokenIn:      "0x3600000000000000000000000000000000000000",
		TokenOut:     "0x3600000000000000000000000000000000000000",
		AmountIn:     big.NewInt(1000000),
		MinAmountOut: big.NewInt(990000),
		Recipient:    "0x3333333333333333333333333333333333333333",
	}
	call, err := payroll.EncodeRouteAndPay(registry, in)
	if err != nil {
		t.Fatal(err)
	}
	wantSel := contracts.Selector4(payroll.SigRouteAndPay)
	if call.Selector() != wantSel {
		t.Fatalf("selector = %x, want %x", call.Selector(), wantSel)
	}
	// Known first 4 bytes from offline keccak of canonical signature.
	sel := call.Selector()
	if hex.EncodeToString(sel[:]) != "8c7c789c" {
		t.Fatalf("unexpected routeAndPay selector %x", sel)
	}
}

func TestPayrollValidationRejections(t *testing.T) {
	registry := contracts.DefaultRegistry()
	base := payroll.BatchMultiTokenOut{
		TokenIn:       "0x3600000000000000000000000000000000000000",
		TokenOuts:     []string{"0x3600000000000000000000000000000000000000"},
		Recipients:    []string{"0x1111111111111111111111111111111111111111"},
		AmountsIn:     []*big.Int{big.NewInt(1000)},
		MinAmountsOut: []*big.Int{big.NewInt(900)},
		ReferenceID:   "ok",
	}

	t.Run("empty recipients", func(t *testing.T) {
		in := base
		in.Recipients = nil
		in.TokenOuts = nil
		in.AmountsIn = nil
		in.MinAmountsOut = nil
		if _, err := payroll.EncodeBatchMultiTokenOut(registry, in); !hasCode(err, apperrors.CodeValidationError) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("mismatched lengths", func(t *testing.T) {
		in := base
		in.AmountsIn = []*big.Int{big.NewInt(1), big.NewInt(2)}
		if _, err := payroll.EncodeBatchMultiTokenOut(registry, in); !hasCode(err, apperrors.CodeValidationError) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("invalid token", func(t *testing.T) {
		in := base
		in.TokenIn = "not-an-address"
		if _, err := payroll.EncodeBatchMultiTokenOut(registry, in); !hasCode(err, apperrors.CodeValidationError) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("invalid recipient", func(t *testing.T) {
		in := base
		in.Recipients = []string{"0xgg"}
		if _, err := payroll.EncodeBatchMultiTokenOut(registry, in); !hasCode(err, apperrors.CodeValidationError) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("zero amount", func(t *testing.T) {
		in := base
		in.AmountsIn = []*big.Int{big.NewInt(0)}
		if _, err := payroll.EncodeBatchMultiTokenOut(registry, in); !hasCode(err, apperrors.CodeValidationError) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("empty reference", func(t *testing.T) {
		in := base
		in.ReferenceID = ""
		if _, err := payroll.EncodeBatchMultiTokenOut(registry, in); !hasCode(err, apperrors.CodeValidationError) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("unsafe reference", func(t *testing.T) {
		in := base
		in.ReferenceID = "bad\nref"
		if _, err := payroll.EncodeBatchMultiTokenOut(registry, in); !hasCode(err, apperrors.CodeValidationError) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestPayrollRegisteredAddressExact(t *testing.T) {
	call, err := payroll.EncodeRouteAndPay(nil, payroll.SinglePayment{
		TokenIn:      "0x3600000000000000000000000000000000000000",
		TokenOut:     "0x3600000000000000000000000000000000000000",
		AmountIn:     big.NewInt(1),
		MinAmountOut: big.NewInt(0),
		Recipient:    "0x3333333333333333333333333333333333333333",
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.To() != contracts.ChecksumAddress(contracts.AddressWizPayPayroll) &&
		!contracts.AddressesEqual(call.To(), contracts.AddressWizPayPayroll) {
		t.Fatalf("To = %q", call.To())
	}
}

func hasCode(err error, code apperrors.Code) bool {
	if err == nil {
		return false
	}
	return apperrors.ToPublic(err).Code == code
}
