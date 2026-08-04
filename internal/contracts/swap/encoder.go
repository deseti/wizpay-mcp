package swap

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

// EncodeExecuteSwap encodes the allowlisted executeSwap function.
//
// The destination address is taken exclusively from the registry deployment.
// Callers cannot supply an alternate contract address or method name.
func EncodeExecuteSwap(registry *contracts.Registry, in ExecuteSwapInput) (contracts.EncodedCall, error) {
	deployment, err := ExpectedDeployment(registry)
	if err != nil {
		return contracts.EncodedCall{}, err
	}
	if err := validateDeployment(deployment); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Swap deployment validation failed.", false, true, true, err)
	}
	if err := validateExecuteSwap(in); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Swap execution input is invalid.", false, true, true, err)
	}
	if !deployment.AllowsExecution(SigExecuteSwap) {
		return contracts.EncodedCall{}, apperrors.New(apperrors.CodeValidationError, "Swap execution function is not allowlisted.", false, true, true)
	}

	method, err := MethodBySignature(SigExecuteSwap)
	if err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeInternalError, "Swap ABI method resolution failed.", false, false, true, err)
	}
	packed, err := method.Inputs.Pack(
		common.HexToAddress(in.Router),
		common.HexToAddress(in.TokenIn),
		common.HexToAddress(in.TokenOut),
		new(big.Int).Set(in.AmountIn),
		new(big.Int).Set(in.MinAmountOut),
		common.HexToAddress(in.Recipient),
		big.NewInt(in.Deadline),
	)
	if err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Swap executeSwap encoding failed.", false, true, true, err)
	}

	if len(method.ID) != 4 {
		return contracts.EncodedCall{}, fmt.Errorf("invalid selector length")
	}
	var sel [4]byte
	copy(sel[:], method.ID)
	if err := RejectAdminSelector(sel); err != nil {
		return contracts.EncodedCall{}, apperrors.Wrap(apperrors.CodeValidationError, "Swap encoder refused a non-allowlisted selector.", false, true, true, err)
	}

	callData := make([]byte, 0, 4+len(packed))
	callData = append(callData, method.ID...)
	callData = append(callData, packed...)
	return contracts.NewEncodedCall(deployment, SigExecuteSwap, sel, callData)
}
