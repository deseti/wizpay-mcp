package payroll

import "math/big"

// SameTokenPayrollInput is the exact calldata shape of
// executeSameTokenPayroll. It does not include transaction value or approvals
// and therefore is not a live execution plan.
type SameTokenPayrollInput struct {
	Token       string
	Recipients  []string
	Amounts     []*big.Int
	ReferenceID string
}

// CrossTokenPayrollInput is the exact calldata shape of
// executeCrossTokenPayroll. Track A intentionally does not model native value
// or ERC-20 approval sequencing.
type CrossTokenPayrollInput struct {
	TokenIn        string
	TokenOut       string
	Recipients     []string
	OutputAmounts  []*big.Int
	GrossInput     *big.Int
	MinTotalOut    *big.Int
	MinHopPriceX36 *big.Int
	Deadline       *big.Int
	ReferenceID    string
}
