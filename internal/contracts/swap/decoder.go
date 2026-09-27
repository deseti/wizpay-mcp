package swap

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// DecodedExecuteSwap is the exact canonical executeSwap calldata material.
type DecodedExecuteSwap struct {
	TokenIn, TokenOut        string
	AmountIn, MinAmountOut   *big.Int
	MinHopPriceX36, Deadline *big.Int
}

func DecodeExecuteSwapCall(data []byte) (DecodedExecuteSwap, error) {
	method, err := MethodBySignature(SigExecuteSwap)
	if err != nil {
		return DecodedExecuteSwap{}, err
	}
	if len(data) < 4 || !bytes.Equal(data[:4], method.ID) {
		return DecodedExecuteSwap{}, fmt.Errorf("calldata selector is not executeSwap")
	}
	values, err := method.Inputs.Unpack(data[4:])
	if err != nil || len(values) != 6 {
		return DecodedExecuteSwap{}, fmt.Errorf("decode executeSwap calldata: %w", err)
	}
	tokenIn, ok1 := values[0].(common.Address)
	tokenOut, ok2 := values[1].(common.Address)
	amountIn, ok3 := values[2].(*big.Int)
	minAmountOut, ok4 := values[3].(*big.Int)
	minHopPrice, ok5 := values[4].(*big.Int)
	deadline, ok6 := values[5].(*big.Int)
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
		return DecodedExecuteSwap{}, fmt.Errorf("executeSwap calldata has unexpected ABI types")
	}
	out := DecodedExecuteSwap{TokenIn: tokenIn.Hex(), TokenOut: tokenOut.Hex(), AmountIn: new(big.Int).Set(amountIn), MinAmountOut: new(big.Int).Set(minAmountOut), MinHopPriceX36: new(big.Int).Set(minHopPrice), Deadline: new(big.Int).Set(deadline)}
	if err := validateExecuteSwap(ExecuteSwapInput{TokenIn: out.TokenIn, TokenOut: out.TokenOut, AmountIn: out.AmountIn, MinAmountOut: out.MinAmountOut, MinHopPriceX36: out.MinHopPriceX36, Deadline: out.Deadline}); err != nil {
		return DecodedExecuteSwap{}, err
	}
	repacked, err := method.Inputs.Pack(tokenIn, tokenOut, amountIn, minAmountOut, minHopPrice, deadline)
	if err != nil || !bytes.Equal(data, append(append([]byte(nil), method.ID...), repacked...)) {
		return DecodedExecuteSwap{}, fmt.Errorf("executeSwap calldata is not canonical")
	}
	return out, nil
}
