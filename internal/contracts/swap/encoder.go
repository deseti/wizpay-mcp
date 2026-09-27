package swap

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

// EncodeExecuteSwap encodes only the reviewed Mainnet calldata shape. It does
// not model native value, ERC-20 approvals, submission, or execution.
func EncodeExecuteSwap(registry *contracts.Registry, in ExecuteSwapInput) (contracts.EncodedCall, error) {
	deployment, err := ExpectedDeployment(registry)
	if err != nil {
		return contracts.EncodedCall{}, err
	}
	if err := validateDeployment(deployment); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Swap deployment validation failed.", false, true, true, err)
	}
	if err := validateExecuteSwap(in); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Swap descriptor is invalid.", false, true, true, err)
	}
	method, err := MethodBySignature(SigExecuteSwap)
	if err != nil {
		return contracts.EncodedCall{}, err
	}
	packed, err := method.Inputs.Pack(
		common.HexToAddress(in.TokenIn), common.HexToAddress(in.TokenOut),
		new(big.Int).Set(in.AmountIn), new(big.Int).Set(in.MinAmountOut),
		new(big.Int).Set(in.MinHopPriceX36), new(big.Int).Set(in.Deadline),
	)
	if err != nil {
		return contracts.EncodedCall{}, fmt.Errorf("encode swap descriptor: %w", err)
	}
	var selector [4]byte
	copy(selector[:], method.ID)
	if err := RejectAdminSelector(selector); err != nil {
		return contracts.EncodedCall{}, err
	}
	callData := append(append(make([]byte, 0, 4+len(packed)), method.ID...), packed...)
	return contracts.NewEncodedCall(deployment, SigExecuteSwap, selector, callData)
}
