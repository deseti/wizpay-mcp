package payroll

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

type PaymentEvent struct {
	ReferenceHash                 [32]byte
	Employer, TokenOut, Recipient string
	PaymentIndex, AmountOut       *big.Int
}

type ReferenceConsumedEvent struct {
	ReferenceHash, BatchDigest                         [32]byte
	Employer, TokenIn, TokenOut                        string
	TotalInput, TotalOutput, TotalFees, RecipientCount *big.Int
	ReferenceID                                        string
}

type BatchExecutedEvent struct {
	Employer, TokenIn, TokenOut                        string
	TotalInput, TotalOutput, TotalFees, RecipientCount *big.Int
	ReferenceID                                        string
}

func DecodePaymentEvent(registry *contracts.Registry, log contracts.Log) (PaymentEvent, error) {
	if err := ValidateEventLog(registry, SigPayrollPayment, log); err != nil {
		return PaymentEvent{}, err
	}
	if len(log.Topics) != 4 {
		return PaymentEvent{}, fmt.Errorf("PayrollPayment requires four topics")
	}
	event, _ := EventBySignature(SigPayrollPayment)
	values, err := event.Inputs.NonIndexed().Unpack(log.Data)
	if err != nil || len(values) != 3 {
		return PaymentEvent{}, fmt.Errorf("decode PayrollPayment: %w", err)
	}
	recipient, ok1 := values[0].(common.Address)
	index, ok2 := values[1].(*big.Int)
	amount, ok3 := values[2].(*big.Int)
	if !ok1 || !ok2 || !ok3 {
		return PaymentEvent{}, fmt.Errorf("PayrollPayment has unexpected ABI types")
	}
	if packed, packErr := event.Inputs.NonIndexed().Pack(values...); packErr != nil || !bytes.Equal(packed, log.Data) {
		return PaymentEvent{}, fmt.Errorf("PayrollPayment data is not canonical")
	}
	return PaymentEvent{ReferenceHash: topicHash(log.Topics[1]), Employer: topicAddress(log.Topics[2]), TokenOut: topicAddress(log.Topics[3]), Recipient: recipient.Hex(), PaymentIndex: new(big.Int).Set(index), AmountOut: new(big.Int).Set(amount)}, nil
}

func DecodeReferenceConsumedEvent(registry *contracts.Registry, log contracts.Log) (ReferenceConsumedEvent, error) {
	if err := ValidateEventLog(registry, SigPayrollReferenceConsumed, log); err != nil {
		return ReferenceConsumedEvent{}, err
	}
	if len(log.Topics) != 4 {
		return ReferenceConsumedEvent{}, fmt.Errorf("PayrollReferenceConsumed requires four topics")
	}
	event, _ := EventBySignature(SigPayrollReferenceConsumed)
	v, err := event.Inputs.NonIndexed().Unpack(log.Data)
	if err != nil || len(v) != 7 {
		return ReferenceConsumedEvent{}, fmt.Errorf("decode PayrollReferenceConsumed: %w", err)
	}
	tokenOut, a := v[0].(common.Address)
	digest, b := v[1].([32]byte)
	totalIn, c := v[2].(*big.Int)
	totalOut, d := v[3].(*big.Int)
	fees, e := v[4].(*big.Int)
	count, f := v[5].(*big.Int)
	ref, g := v[6].(string)
	if !a || !b || !c || !d || !e || !f || !g {
		return ReferenceConsumedEvent{}, fmt.Errorf("PayrollReferenceConsumed has unexpected ABI types")
	}
	if packed, packErr := event.Inputs.NonIndexed().Pack(v...); packErr != nil || !bytes.Equal(packed, log.Data) {
		return ReferenceConsumedEvent{}, fmt.Errorf("PayrollReferenceConsumed data is not canonical")
	}
	return ReferenceConsumedEvent{ReferenceHash: topicHash(log.Topics[1]), Employer: topicAddress(log.Topics[2]), TokenIn: topicAddress(log.Topics[3]), TokenOut: tokenOut.Hex(), BatchDigest: digest, TotalInput: cloneInt(totalIn), TotalOutput: cloneInt(totalOut), TotalFees: cloneInt(fees), RecipientCount: cloneInt(count), ReferenceID: ref}, nil
}

func DecodeBatchExecutedEvent(registry *contracts.Registry, log contracts.Log) (BatchExecutedEvent, error) {
	if err := ValidateEventLog(registry, SigPayrollBatchExecuted, log); err != nil {
		return BatchExecutedEvent{}, err
	}
	if len(log.Topics) != 4 {
		return BatchExecutedEvent{}, fmt.Errorf("PayrollBatchExecuted requires four topics")
	}
	event, _ := EventBySignature(SigPayrollBatchExecuted)
	v, err := event.Inputs.NonIndexed().Unpack(log.Data)
	if err != nil || len(v) != 5 {
		return BatchExecutedEvent{}, fmt.Errorf("decode PayrollBatchExecuted: %w", err)
	}
	totalIn, a := v[0].(*big.Int)
	totalOut, b := v[1].(*big.Int)
	fees, c := v[2].(*big.Int)
	count, d := v[3].(*big.Int)
	ref, e := v[4].(string)
	if !a || !b || !c || !d || !e {
		return BatchExecutedEvent{}, fmt.Errorf("PayrollBatchExecuted has unexpected ABI types")
	}
	if packed, packErr := event.Inputs.NonIndexed().Pack(v...); packErr != nil || !bytes.Equal(packed, log.Data) {
		return BatchExecutedEvent{}, fmt.Errorf("PayrollBatchExecuted data is not canonical")
	}
	return BatchExecutedEvent{Employer: topicAddress(log.Topics[1]), TokenIn: topicAddress(log.Topics[2]), TokenOut: topicAddress(log.Topics[3]), TotalInput: cloneInt(totalIn), TotalOutput: cloneInt(totalOut), TotalFees: cloneInt(fees), RecipientCount: cloneInt(count), ReferenceID: ref}, nil
}

func topicHash(topic []byte) (out [32]byte) {
	if len(topic) == 32 {
		copy(out[:], topic)
	}
	return
}
func topicAddress(topic []byte) string {
	if len(topic) != 32 {
		return ""
	}
	return common.BytesToAddress(topic[12:]).Hex()
}
func cloneInt(v *big.Int) *big.Int { return new(big.Int).Set(v) }
