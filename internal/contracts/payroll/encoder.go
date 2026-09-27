package payroll

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

// EncodeSameTokenPayroll encodes only the reviewed Mainnet calldata shape.
// It does not provide approvals, transaction value, submission, or execution.
func EncodeSameTokenPayroll(registry *contracts.Registry, in SameTokenPayrollInput) (contracts.EncodedCall, error) {
	deployment, err := ExpectedDeployment(registry)
	if err != nil {
		return contracts.EncodedCall{}, err
	}
	if err := validateDeployment(deployment); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll deployment validation failed.", false, true, true, err)
	}
	if err := validateSameToken(in); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Same-token payroll descriptor is invalid.", false, true, true, err)
	}
	method, err := MethodBySignature(SigExecuteSameTokenPayroll)
	if err != nil {
		return contracts.EncodedCall{}, err
	}
	packed, err := method.Inputs.Pack(common.HexToAddress(in.Token), toAddresses(in.Recipients), copyBigInts(in.Amounts), in.ReferenceID)
	if err != nil {
		return contracts.EncodedCall{}, fmt.Errorf("encode same-token payroll descriptor: %w", err)
	}
	return buildCall(deployment, SigExecuteSameTokenPayroll, method.ID, packed)
}

// EncodeCrossTokenPayroll encodes only the reviewed Mainnet calldata shape.
// Its output is not executable without later-track native-value or approval
// handling selected from the fixed canonical token direction.
func EncodeCrossTokenPayroll(registry *contracts.Registry, in CrossTokenPayrollInput) (contracts.EncodedCall, error) {
	deployment, err := ExpectedDeployment(registry)
	if err != nil {
		return contracts.EncodedCall{}, err
	}
	if err := validateDeployment(deployment); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll deployment validation failed.", false, true, true, err)
	}
	if err := validateCrossToken(in); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Cross-token payroll descriptor is invalid.", false, true, true, err)
	}
	method, err := MethodBySignature(SigExecuteCrossTokenPayroll)
	if err != nil {
		return contracts.EncodedCall{}, err
	}
	packed, err := method.Inputs.Pack(
		common.HexToAddress(in.TokenIn), common.HexToAddress(in.TokenOut),
		toAddresses(in.Recipients), copyBigInts(in.OutputAmounts),
		new(big.Int).Set(in.GrossInput), new(big.Int).Set(in.MinTotalOut),
		new(big.Int).Set(in.MinHopPriceX36), new(big.Int).Set(in.Deadline), in.ReferenceID,
	)
	if err != nil {
		return contracts.EncodedCall{}, fmt.Errorf("encode cross-token payroll descriptor: %w", err)
	}
	return buildCall(deployment, SigExecuteCrossTokenPayroll, method.ID, packed)
}

func buildCall(deployment contracts.Deployment, signature string, selector, packed []byte) (contracts.EncodedCall, error) {
	if len(selector) != 4 {
		return contracts.EncodedCall{}, fmt.Errorf("invalid selector length")
	}
	var sel [4]byte
	copy(sel[:], selector)
	if err := RejectAdminSelector(sel); err != nil {
		return contracts.EncodedCall{}, err
	}
	callData := append(append(make([]byte, 0, 4+len(packed)), selector...), packed...)
	return contracts.NewEncodedCall(deployment, signature, sel, callData)
}

func toAddresses(values []string) []common.Address {
	out := make([]common.Address, len(values))
	for i, value := range values {
		out[i] = common.HexToAddress(value)
	}
	return out
}

func copyBigInts(values []*big.Int) []*big.Int {
	out := make([]*big.Int, len(values))
	for i, value := range values {
		out[i] = new(big.Int).Set(value)
	}
	return out
}
