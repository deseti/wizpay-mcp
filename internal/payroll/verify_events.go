package payroll

import (
	"fmt"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractpayroll "github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

func batchUnprovableFields(variant intents.PayrollVariant) []string {
	fields := []string{
		"per_recipient_address",
		"per_recipient_amount_in",
		"per_recipient_min_amount_out",
		"per_recipient_amount_out",
	}
	if variant == intents.PayrollVariantBatchMultiTokenOut {
		fields = append(fields, "per_recipient_token_out", "aggregate_token_out_meaning")
	}
	return fields
}

func (v Verifier) collectPaymentRouted(receipt providers.Receipt) (matches []contractpayroll.PaymentRouted, ambiguous bool, err error) {
	var decoded []contractpayroll.PaymentRouted
	var malformed int
	for _, log := range receipt.Logs {
		// Wrong contract: ignore (not a match). Same signature from another
		// address must never count.
		if !contracts.ValidAddress(log.Address) || !contracts.AddressesEqual(log.Address, contracts.AddressWizPayPayroll) {
			continue
		}
		if len(log.Topics) == 0 {
			continue
		}
		topic0, terr := contractpayroll.EventTopic0(contractpayroll.SigPaymentRouted)
		if terr != nil {
			return nil, false, terr
		}
		if !topicsEqual(log.Topics[0], topic0) {
			continue
		}
		event, derr := contractpayroll.DecodePaymentRouted(v.registry, log.ContractLog(receipt.ChainID))
		if derr != nil {
			malformed++
			continue
		}
		decoded = append(decoded, event)
	}
	if malformed > 0 && len(decoded) == 0 {
		return nil, false, fmt.Errorf("malformed PaymentRouted log")
	}
	if malformed > 0 && len(decoded) > 0 {
		// Mix of good and bad candidate logs is ambiguous.
		return decoded, true, nil
	}
	return decoded, false, nil
}

func (v Verifier) collectBatchPaymentRouted(receipt providers.Receipt) (matches []contractpayroll.BatchPaymentRouted, ambiguous bool, err error) {
	var decoded []contractpayroll.BatchPaymentRouted
	var malformed int
	for _, log := range receipt.Logs {
		if !contracts.ValidAddress(log.Address) || !contracts.AddressesEqual(log.Address, contracts.AddressWizPayPayroll) {
			continue
		}
		if len(log.Topics) == 0 {
			continue
		}
		topic0, terr := contractpayroll.EventTopic0(contractpayroll.SigBatchPaymentRouted)
		if terr != nil {
			return nil, false, terr
		}
		if !topicsEqual(log.Topics[0], topic0) {
			continue
		}
		event, derr := contractpayroll.DecodeBatchPaymentRouted(v.registry, log.ContractLog(receipt.ChainID))
		if derr != nil {
			malformed++
			continue
		}
		decoded = append(decoded, event)
	}
	if malformed > 0 && len(decoded) == 0 {
		return nil, false, fmt.Errorf("malformed BatchPaymentRouted log")
	}
	if malformed > 0 && len(decoded) > 0 {
		return decoded, true, nil
	}
	return decoded, false, nil
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
