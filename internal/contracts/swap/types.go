package swap

import (
	"math/big"
)

// ExecuteSwapInput is an already-approved immutable swap execution requirement.
//
// The contract layer does not choose the router, fetch quotes, calculate
// business-level slippage, or alter recipient/token/amount values. All fields
// must already come from a higher-level approved immutable plan.
type ExecuteSwapInput struct {
	Router       string
	TokenIn      string
	TokenOut     string
	AmountIn     *big.Int
	MinAmountOut *big.Int
	Recipient    string
	// Deadline is a unix timestamp (seconds). Encoding fails if the deadline
	// is zero or has already expired at encoding time.
	Deadline int64
}
