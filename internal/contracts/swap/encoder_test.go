package swap_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/contracts/swap"
)

func TestMainnetSwapDescriptorEncodingParity(t *testing.T) {
	in := swap.ExecuteSwapInput{TokenIn: contracts.AddressEURCMainnet, TokenOut: contracts.AddressUSDCMainnet, AmountIn: big.NewInt(1_000_000), MinAmountOut: big.NewInt(990_000), MinHopPriceX36: big.NewInt(1), Deadline: big.NewInt(1)}
	first, err := swap.EncodeExecuteSwap(nil, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := swap.EncodeExecuteSwap(nil, in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.CallData(), second.CallData()) {
		t.Fatal("swap encoding is not deterministic")
	}
	if first.Function() != swap.SigExecuteSwap || first.Selector() != contracts.Selector4(swap.SigExecuteSwap) {
		t.Fatal("swap selector mismatch")
	}
	if got := first.Selector(); got != [4]byte{0x66, 0xa3, 0xbe, 0xe6} {
		t.Fatalf("swap selector = 0x%x", got)
	}
	if first.ChainID() != contracts.ChainIDArcMainnet || first.Network() != contracts.NetworkArcMainnet || !contracts.AddressesEqual(first.To(), contracts.AddressWizPaySwapExecutor) {
		t.Fatal("swap descriptor identity mismatch")
	}
}

func TestMainnetSwapDescriptorRejectsArbitraryTokenAndOldShape(t *testing.T) {
	_, err := swap.EncodeExecuteSwap(nil, swap.ExecuteSwapInput{TokenIn: "0x1111111111111111111111111111111111111111", TokenOut: contracts.AddressUSDCMainnet, AmountIn: big.NewInt(1), MinAmountOut: big.NewInt(1), MinHopPriceX36: big.NewInt(1), Deadline: big.NewInt(1)})
	if err == nil {
		t.Fatal("unsupported token must fail closed")
	}
	if _, err := swap.MethodBySignature("executeSwap(address,address,address,uint256,uint256,address,uint256)"); err == nil {
		t.Fatal("Testnet swap shape must not remain in runtime ABI")
	}
}
