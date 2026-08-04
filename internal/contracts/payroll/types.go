package payroll

import (
	"math/big"
)

// BatchMultiTokenOut is an already-approved immutable batch payroll execution
// requirement using the multi-token-out overload of batchRouteAndPay.
//
// This type does not plan payroll: recipients, tokens, and amounts must already
// come from a higher-level approved immutable plan. The contract layer only
// validates shape and encodes calldata.
type BatchMultiTokenOut struct {
	TokenIn       string
	TokenOuts     []string
	Recipients    []string
	AmountsIn     []*big.Int
	MinAmountsOut []*big.Int
	ReferenceID   string
}

// BatchSingleTokenOut is an already-approved immutable batch payroll execution
// requirement using the single-token-out overload of batchRouteAndPay.
type BatchSingleTokenOut struct {
	TokenIn       string
	TokenOut      string
	Recipients    []string
	AmountsIn     []*big.Int
	MinAmountsOut []*big.Int
	ReferenceID   string
}

// SinglePayment is an already-approved immutable single routeAndPay requirement.
type SinglePayment struct {
	TokenIn      string
	TokenOut     string
	AmountIn     *big.Int
	MinAmountOut *big.Int
	Recipient    string
}
