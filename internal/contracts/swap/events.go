package swap

import (
	"fmt"
	"math/big"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

type WizPayMainnetSwapExecuted struct {
	Caller       string
	TokenIn      string
	TokenOut     string
	AmountIn     *big.Int
	FeeAmount    *big.Int
	NetAmountIn  *big.Int
	AmountOut    *big.Int
	MinAmountOut *big.Int
}

func DecodeWizPayMainnetSwapExecuted(registry *contracts.Registry, log contracts.Log) (WizPayMainnetSwapExecuted, error) {
	deployment, err := ExpectedDeployment(registry)
	if err != nil {
		return WizPayMainnetSwapExecuted{}, err
	}
	if !deployment.AllowsEvent(SigWizPayMainnetSwapExecuted) || (log.ChainID != "" && log.ChainID != deployment.ChainID) || !contracts.AddressesEqual(log.Address, deployment.Address) || len(log.Topics) != 4 {
		return WizPayMainnetSwapExecuted{}, apperrors.New(apperrors.CodeValidationError, "WizPayMainnetSwapExecuted log context is invalid.", false, true, true)
	}
	event, err := EventBySignature(SigWizPayMainnetSwapExecuted)
	if err != nil {
		return WizPayMainnetSwapExecuted{}, err
	}
	if !bytesEqual(log.Topics[0], event.ID.Bytes()) {
		return WizPayMainnetSwapExecuted{}, fmt.Errorf("swap event topic mismatch")
	}
	caller, ok := contracts.AddressFromTopic(log.Topics[1])
	if !ok {
		return WizPayMainnetSwapExecuted{}, fmt.Errorf("caller topic is malformed")
	}
	tokenIn, ok := contracts.AddressFromTopic(log.Topics[2])
	if !ok {
		return WizPayMainnetSwapExecuted{}, fmt.Errorf("tokenIn topic is malformed")
	}
	tokenOut, ok := contracts.AddressFromTopic(log.Topics[3])
	if !ok {
		return WizPayMainnetSwapExecuted{}, fmt.Errorf("tokenOut topic is malformed")
	}
	values, err := event.Inputs.NonIndexed().Unpack(log.Data)
	if err != nil || len(values) != 5 {
		return WizPayMainnetSwapExecuted{}, fmt.Errorf("swap event data is malformed")
	}
	amounts := make([]*big.Int, 5)
	for i, value := range values {
		amount, ok := value.(*big.Int)
		if !ok || amount == nil {
			return WizPayMainnetSwapExecuted{}, fmt.Errorf("swap event amount %d is malformed", i)
		}
		amounts[i] = new(big.Int).Set(amount)
	}
	return WizPayMainnetSwapExecuted{Caller: caller, TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: amounts[0], FeeAmount: amounts[1], NetAmountIn: amounts[2], AmountOut: amounts[3], MinAmountOut: amounts[4]}, nil
}

func EventTopic0() ([]byte, error) {
	event, err := EventBySignature(SigWizPayMainnetSwapExecuted)
	if err != nil {
		return nil, err
	}
	return event.ID.Bytes(), nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
