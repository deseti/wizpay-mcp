package payroll

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

// BatchPaymentRouted is the typed verification event for batch payroll routing.
//
// Indexed/non-indexed layout is derived from contracts/abi/WizPay.json:
//   - indexed: sender
//   - non-indexed: tokenIn, tokenOut, totalAmountIn, totalAmountOut, totalFees,
//     recipientCount, referenceId
//
// Successful decoding is observation evidence only. It does not mark financial
// success; Phase 12 defines domain verification semantics.
type BatchPaymentRouted struct {
	Sender         string
	TokenIn        string
	TokenOut       string
	TotalAmountIn  *big.Int
	TotalAmountOut *big.Int
	TotalFees      *big.Int
	RecipientCount *big.Int
	ReferenceID    string
}

// PaymentRouted is the typed verification event for a single routed payment.
//
// Indexed/non-indexed layout is derived from contracts/abi/WizPay.json:
//   - indexed: sender, recipient
//   - non-indexed: tokenIn, tokenOut, amountIn, amountOut, feeAmount
type PaymentRouted struct {
	Sender    string
	Recipient string
	TokenIn   string
	TokenOut  string
	AmountIn  *big.Int
	AmountOut *big.Int
	FeeAmount *big.Int
}

// DecodeBatchPaymentRouted decodes a BatchPaymentRouted log against the
// registered Payroll deployment. Malformed logs and wrong contract addresses
// fail closed.
func DecodeBatchPaymentRouted(registry *contracts.Registry, log contracts.Log) (BatchPaymentRouted, error) {
	deployment, err := ExpectedDeployment(registry)
	if err != nil {
		return BatchPaymentRouted{}, err
	}
	if err := validateLogContext(deployment, log, SigBatchPaymentRouted, 2); err != nil {
		return BatchPaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "BatchPaymentRouted log is invalid.", false, true, true, err)
	}

	event, err := EventBySignature(SigBatchPaymentRouted)
	if err != nil {
		return BatchPaymentRouted{}, apperrors.Wrap(apperrors.CodeInternalError, "Payroll event resolution failed.", false, false, true, err)
	}
	if !topicsEqual(log.Topics[0], event.ID.Bytes()) {
		return BatchPaymentRouted{}, apperrors.New(apperrors.CodeValidationError, "BatchPaymentRouted topic mismatch.", false, true, true)
	}

	sender, ok := contracts.AddressFromTopic(log.Topics[1])
	if !ok {
		return BatchPaymentRouted{}, apperrors.New(apperrors.CodeValidationError, "BatchPaymentRouted sender topic is malformed.", false, true, true)
	}

	values, err := event.Inputs.NonIndexed().Unpack(log.Data)
	if err != nil {
		return BatchPaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "BatchPaymentRouted data decoding failed.", false, true, true, err)
	}
	if len(values) != 7 {
		return BatchPaymentRouted{}, apperrors.New(apperrors.CodeValidationError, "BatchPaymentRouted data field count mismatch.", false, true, true)
	}

	tokenIn, err := asAddress(values[0])
	if err != nil {
		return BatchPaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "BatchPaymentRouted tokenIn is invalid.", false, true, true, err)
	}
	tokenOut, err := asAddress(values[1])
	if err != nil {
		return BatchPaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "BatchPaymentRouted tokenOut is invalid.", false, true, true, err)
	}
	totalIn, err := asBigInt(values[2])
	if err != nil {
		return BatchPaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "BatchPaymentRouted totalAmountIn is invalid.", false, true, true, err)
	}
	totalOut, err := asBigInt(values[3])
	if err != nil {
		return BatchPaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "BatchPaymentRouted totalAmountOut is invalid.", false, true, true, err)
	}
	totalFees, err := asBigInt(values[4])
	if err != nil {
		return BatchPaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "BatchPaymentRouted totalFees is invalid.", false, true, true, err)
	}
	recipientCount, err := asBigInt(values[5])
	if err != nil {
		return BatchPaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "BatchPaymentRouted recipientCount is invalid.", false, true, true, err)
	}
	referenceID, err := asString(values[6])
	if err != nil {
		return BatchPaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "BatchPaymentRouted referenceId is invalid.", false, true, true, err)
	}

	return BatchPaymentRouted{
		Sender:         sender,
		TokenIn:        tokenIn,
		TokenOut:       tokenOut,
		TotalAmountIn:  totalIn,
		TotalAmountOut: totalOut,
		TotalFees:      totalFees,
		RecipientCount: recipientCount,
		ReferenceID:    referenceID,
	}, nil
}

// DecodePaymentRouted decodes a PaymentRouted log against the registered
// Payroll deployment.
func DecodePaymentRouted(registry *contracts.Registry, log contracts.Log) (PaymentRouted, error) {
	deployment, err := ExpectedDeployment(registry)
	if err != nil {
		return PaymentRouted{}, err
	}
	if err := validateLogContext(deployment, log, SigPaymentRouted, 3); err != nil {
		return PaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "PaymentRouted log is invalid.", false, true, true, err)
	}

	event, err := EventBySignature(SigPaymentRouted)
	if err != nil {
		return PaymentRouted{}, apperrors.Wrap(apperrors.CodeInternalError, "Payroll event resolution failed.", false, false, true, err)
	}
	if !topicsEqual(log.Topics[0], event.ID.Bytes()) {
		return PaymentRouted{}, apperrors.New(apperrors.CodeValidationError, "PaymentRouted topic mismatch.", false, true, true)
	}

	sender, ok := contracts.AddressFromTopic(log.Topics[1])
	if !ok {
		return PaymentRouted{}, apperrors.New(apperrors.CodeValidationError, "PaymentRouted sender topic is malformed.", false, true, true)
	}
	recipient, ok := contracts.AddressFromTopic(log.Topics[2])
	if !ok {
		return PaymentRouted{}, apperrors.New(apperrors.CodeValidationError, "PaymentRouted recipient topic is malformed.", false, true, true)
	}

	values, err := event.Inputs.NonIndexed().Unpack(log.Data)
	if err != nil {
		return PaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "PaymentRouted data decoding failed.", false, true, true, err)
	}
	if len(values) != 5 {
		return PaymentRouted{}, apperrors.New(apperrors.CodeValidationError, "PaymentRouted data field count mismatch.", false, true, true)
	}

	tokenIn, err := asAddress(values[0])
	if err != nil {
		return PaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "PaymentRouted tokenIn is invalid.", false, true, true, err)
	}
	tokenOut, err := asAddress(values[1])
	if err != nil {
		return PaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "PaymentRouted tokenOut is invalid.", false, true, true, err)
	}
	amountIn, err := asBigInt(values[2])
	if err != nil {
		return PaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "PaymentRouted amountIn is invalid.", false, true, true, err)
	}
	amountOut, err := asBigInt(values[3])
	if err != nil {
		return PaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "PaymentRouted amountOut is invalid.", false, true, true, err)
	}
	feeAmount, err := asBigInt(values[4])
	if err != nil {
		return PaymentRouted{}, apperrors.Wrap(apperrors.CodeValidationError, "PaymentRouted feeAmount is invalid.", false, true, true, err)
	}

	return PaymentRouted{
		Sender:    sender,
		Recipient: recipient,
		TokenIn:   tokenIn,
		TokenOut:  tokenOut,
		AmountIn:  amountIn,
		AmountOut: amountOut,
		FeeAmount: feeAmount,
	}, nil
}

// EventTopic0 returns the topic0 hash for a canonical payroll verification event.
func EventTopic0(signature string) ([]byte, error) {
	event, err := EventBySignature(signature)
	if err != nil {
		return nil, err
	}
	return event.ID.Bytes(), nil
}

func validateLogContext(deployment contracts.Deployment, log contracts.Log, signature string, minTopics int) error {
	if !deployment.AllowsEvent(signature) {
		return fmt.Errorf("event is not allowlisted for payroll verification")
	}
	if log.ChainID != "" && log.ChainID != deployment.ChainID {
		return fmt.Errorf("log chain ID does not match payroll deployment")
	}
	if !contracts.ValidAddress(log.Address) || !contracts.AddressesEqual(log.Address, deployment.Address) {
		return fmt.Errorf("log address does not match registered payroll contract")
	}
	if len(log.Topics) < minTopics {
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

func asString(value any) (string, error) {
	typed, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("expected string, got %T", value)
	}
	return typed, nil
}
