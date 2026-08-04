package payroll

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

// EncodeBatchMultiTokenOut encodes the multi-token-out batchRouteAndPay overload.
//
// The destination address is taken exclusively from the registry deployment.
// Callers cannot supply an alternate contract address or method name.
func EncodeBatchMultiTokenOut(registry *contracts.Registry, in BatchMultiTokenOut) (contracts.EncodedCall, error) {
	deployment, err := ExpectedDeployment(registry)
	if err != nil {
		return contracts.EncodedCall{}, err
	}
	if err := validateDeployment(deployment); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll deployment validation failed.", false, true, true, err)
	}
	if err := validateBatchMulti(in); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll batch multi-token input is invalid.", false, true, true, err)
	}
	if !deployment.AllowsExecution(SigBatchMultiTokenOut) {
		return contracts.EncodedCall{}, apperrors.New(apperrors.CodeValidationError, "Payroll execution function is not allowlisted.", false, true, true)
	}

	method, err := MethodBySignature(SigBatchMultiTokenOut)
	if err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeInternalError, "Payroll ABI method resolution failed.", false, false, true, err)
	}
	packed, err := method.Inputs.Pack(
		common.HexToAddress(in.TokenIn),
		toAddresses(in.TokenOuts),
		toAddresses(in.Recipients),
		copyBigInts(in.AmountsIn),
		copyBigInts(in.MinAmountsOut),
		in.ReferenceID,
	)
	if err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll batch multi-token encoding failed.", false, true, true, err)
	}
	return buildCall(deployment, SigBatchMultiTokenOut, method.ID, packed)
}

// EncodeBatchSingleTokenOut encodes the single-token-out batchRouteAndPay overload.
func EncodeBatchSingleTokenOut(registry *contracts.Registry, in BatchSingleTokenOut) (contracts.EncodedCall, error) {
	deployment, err := ExpectedDeployment(registry)
	if err != nil {
		return contracts.EncodedCall{}, err
	}
	if err := validateDeployment(deployment); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll deployment validation failed.", false, true, true, err)
	}
	if err := validateBatchSingle(in); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll batch single-token input is invalid.", false, true, true, err)
	}
	if !deployment.AllowsExecution(SigBatchSingleTokenOut) {
		return contracts.EncodedCall{}, apperrors.New(apperrors.CodeValidationError, "Payroll execution function is not allowlisted.", false, true, true)
	}

	method, err := MethodBySignature(SigBatchSingleTokenOut)
	if err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeInternalError, "Payroll ABI method resolution failed.", false, false, true, err)
	}
	packed, err := method.Inputs.Pack(
		common.HexToAddress(in.TokenIn),
		common.HexToAddress(in.TokenOut),
		toAddresses(in.Recipients),
		copyBigInts(in.AmountsIn),
		copyBigInts(in.MinAmountsOut),
		in.ReferenceID,
	)
	if err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll batch single-token encoding failed.", false, true, true, err)
	}
	return buildCall(deployment, SigBatchSingleTokenOut, method.ID, packed)
}

// EncodeRouteAndPay encodes the single routeAndPay function.
func EncodeRouteAndPay(registry *contracts.Registry, in SinglePayment) (contracts.EncodedCall, error) {
	deployment, err := ExpectedDeployment(registry)
	if err != nil {
		return contracts.EncodedCall{}, err
	}
	if err := validateDeployment(deployment); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll deployment validation failed.", false, true, true, err)
	}
	if err := validateSingle(in); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll single payment input is invalid.", false, true, true, err)
	}
	if !deployment.AllowsExecution(SigRouteAndPay) {
		return contracts.EncodedCall{}, apperrors.New(apperrors.CodeValidationError, "Payroll execution function is not allowlisted.", false, true, true)
	}

	method, err := MethodBySignature(SigRouteAndPay)
	if err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeInternalError, "Payroll ABI method resolution failed.", false, false, true, err)
	}
	packed, err := method.Inputs.Pack(
		common.HexToAddress(in.TokenIn),
		common.HexToAddress(in.TokenOut),
		new(big.Int).Set(in.AmountIn),
		new(big.Int).Set(in.MinAmountOut),
		common.HexToAddress(in.Recipient),
	)
	if err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll single payment encoding failed.", false, true, true, err)
	}
	return buildCall(deployment, SigRouteAndPay, method.ID, packed)
}

func buildCall(deployment contracts.Deployment, signature string, selector []byte, packed []byte) (contracts.EncodedCall, error) {
	if len(selector) != 4 {
		return contracts.EncodedCall{}, fmt.Errorf("invalid selector length")
	}
	var sel [4]byte
	copy(sel[:], selector)
	if err := RejectAdminSelector(sel); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll encoder refused a non-allowlisted selector.", false, true, true, err)
	}
	callData := make([]byte, 0, 4+len(packed))
	callData = append(callData, selector...)
	callData = append(callData, packed...)
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
