package swap

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

// WizPaySwapExecuted is the typed verification event for swap execution.
//
// Indexed/non-indexed layout is derived from contracts/abi/WizPaySwapExecutor.json:
//   - indexed: user, router, tokenIn
//   - non-indexed: tokenOut, amountIn, feeAmount, netAmountIn, amountOut, recipient
//
// Successful decoding is observation evidence only. It does not mark financial
// success; Phase 12 defines domain verification semantics.
type WizPaySwapExecuted struct {
	User        string
	Router      string
	TokenIn     string
	TokenOut    string
	AmountIn    *big.Int
	FeeAmount   *big.Int
	NetAmountIn *big.Int
	AmountOut   *big.Int
	Recipient   string
}

// DecodeWizPaySwapExecuted decodes a WizPaySwapExecuted log against the
// registered Swap deployment. Malformed logs and wrong contract addresses
// fail closed.
func DecodeWizPaySwapExecuted(registry *contracts.Registry, log contracts.Log) (WizPaySwapExecuted, error) {
	deployment, err := ExpectedDeployment(registry)
	if err != nil {
		return WizPaySwapExecuted{}, err
	}
	if err := validateLogContext(deployment, log); err != nil {
		return WizPaySwapExecuted{}, apperrors.Wrap(apperrors.CodeValidationError, "WizPaySwapExecuted log is invalid.", false, true, true, err)
	}

	event, err := EventBySignature(SigWizPaySwapExecuted)
	if err != nil {
		return WizPaySwapExecuted{}, apperrors.Wrap(apperrors.CodeInternalError, "Swap event resolution failed.", false, false, true, err)
	}
	if !topicsEqual(log.Topics[0], event.ID.Bytes()) {
		return WizPaySwapExecuted{}, apperrors.New(apperrors.CodeValidationError, "WizPaySwapExecuted topic mismatch.", false, true, true)
	}

	user, ok := contracts.AddressFromTopic(log.Topics[1])
	if !ok {
		return WizPaySwapExecuted{}, apperrors.New(apperrors.CodeValidationError, "WizPaySwapExecuted user topic is malformed.", false, true, true)
	}
	router, ok := contracts.AddressFromTopic(log.Topics[2])
	if !ok {
		return WizPaySwapExecuted{}, apperrors.New(apperrors.CodeValidationError, "WizPaySwapExecuted router topic is malformed.", false, true, true)
	}
	tokenIn, ok := contracts.AddressFromTopic(log.Topics[3])
	if !ok {
		return WizPaySwapExecuted{}, apperrors.New(apperrors.CodeValidationError, "WizPaySwapExecuted tokenIn topic is malformed.", false, true, true)
	}

	values, err := event.Inputs.NonIndexed().Unpack(log.Data)
	if err != nil {
		return WizPaySwapExecuted{}, apperrors.Wrap(apperrors.CodeValidationError, "WizPaySwapExecuted data decoding failed.", false, true, true, err)
	}
	if len(values) != 6 {
		return WizPaySwapExecuted{}, apperrors.New(apperrors.CodeValidationError, "WizPaySwapExecuted data field count mismatch.", false, true, true)
	}

	tokenOut, err := asAddress(values[0])
	if err != nil {
		return WizPaySwapExecuted{}, apperrors.Wrap(apperrors.CodeValidationError, "WizPaySwapExecuted tokenOut is invalid.", false, true, true, err)
	}
	amountIn, err := asBigInt(values[1])
	if err != nil {
		return WizPaySwapExecuted{}, apperrors.Wrap(apperrors.CodeValidationError, "WizPaySwapExecuted amountIn is invalid.", false, true, true, err)
	}
	feeAmount, err := asBigInt(values[2])
	if err != nil {
		return WizPaySwapExecuted{}, apperrors.Wrap(apperrors.CodeValidationError, "WizPaySwapExecuted feeAmount is invalid.", false, true, true, err)
	}
	netAmountIn, err := asBigInt(values[3])
	if err != nil {
		return WizPaySwapExecuted{}, apperrors.Wrap(apperrors.CodeValidationError, "WizPaySwapExecuted netAmountIn is invalid.", false, true, true, err)
	}
	amountOut, err := asBigInt(values[4])
	if err != nil {
		return WizPaySwapExecuted{}, apperrors.Wrap(apperrors.CodeValidationError, "WizPaySwapExecuted amountOut is invalid.", false, true, true, err)
	}
	recipient, err := asAddress(values[5])
	if err != nil {
		return WizPaySwapExecuted{}, apperrors.Wrap(apperrors.CodeValidationError, "WizPaySwapExecuted recipient is invalid.", false, true, true, err)
	}

	return WizPaySwapExecuted{
		User:        user,
		Router:      router,
		TokenIn:     tokenIn,
		TokenOut:    tokenOut,
		AmountIn:    amountIn,
		FeeAmount:   feeAmount,
		NetAmountIn: netAmountIn,
		AmountOut:   amountOut,
		Recipient:   recipient,
	}, nil
}

// EventTopic0 returns the topic0 hash for the swap verification event.
func EventTopic0() ([]byte, error) {
	event, err := EventBySignature(SigWizPaySwapExecuted)
	if err != nil {
		return nil, err
	}
	return event.ID.Bytes(), nil
}

func validateLogContext(deployment contracts.Deployment, log contracts.Log) error {
	if !deployment.AllowsEvent(SigWizPaySwapExecuted) {
		return fmt.Errorf("event is not allowlisted for swap verification")
	}
	if log.ChainID != "" && log.ChainID != deployment.ChainID {
		return fmt.Errorf("log chain ID does not match swap deployment")
	}
	if !contracts.ValidAddress(log.Address) || !contracts.AddressesEqual(log.Address, deployment.Address) {
		return fmt.Errorf("log address does not match registered swap contract")
	}
	if len(log.Topics) < 4 {
		return fmt.Errorf("log topic count is insufficient")
	}
	if len(log.Topics[0]) != 32 {
		return fmt.Errorf("log topic0 is malformed")
	}
	return nil
}

func topicsEqual(a, b []byte) bool {
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

func asAddress(value any) (string, error) {
	switch typed := value.(type) {
	case common.Address:
		return typed.Hex(), nil
	default:
		return "", fmt.Errorf("expected address, got %T", value)
	}
}

func asBigInt(value any) (*big.Int, error) {
	switch typed := value.(type) {
	case *big.Int:
		if typed == nil {
			return nil, fmt.Errorf("nil uint256")
		}
		return new(big.Int).Set(typed), nil
	default:
		return nil, fmt.Errorf("expected *big.Int, got %T", value)
	}
}
