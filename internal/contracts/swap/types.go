package swap

import (
	"math/big"
)

// ExecuteSwapInput is the exact calldata shape of the Mainnet executeSwap
// descriptor. Router and recipient are contract semantics and cannot be
// supplied by MCP. Native value is derived separately from this sealed call.
type ExecuteSwapInput struct {
	TokenIn        string
	TokenOut       string
	AmountIn       *big.Int
	MinAmountOut   *big.Int
	MinHopPriceX36 *big.Int
	Deadline       *big.Int
}
