package payroll

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractpayroll "github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

type DomainStatus string

const (
	DomainVerified      DomainStatus = "DOMAIN_VERIFIED"
	DomainAggregateOnly DomainStatus = "DOMAIN_AGGREGATE_ONLY"
	DomainUnverified    DomainStatus = "DOMAIN_UNVERIFIED"
	DomainFailed        DomainStatus = "DOMAIN_FAILED"
)

type DomainResult struct {
	Status                     DomainStatus
	ReasonCode, EventSignature string
	Provable, Unprovable       []string
	TransactionHash            string
	definitiveFailure          bool
}

func (r DomainResult) DefinitiveFailure() bool {
	return r.Status == DomainFailed && r.definitiveFailure
}
func (r DomainResult) FinancialComplete() bool { return r.Status == DomainVerified }

type Verifier struct{ registry *contracts.Registry }

func NewVerifier(registry *contracts.Registry) Verifier { return Verifier{registry: registry} }

func (v Verifier) Verify(intent intents.Intent, plan Plan, receipt providers.Receipt) (DomainResult, error) {
	if err := intent.Validate(); err != nil {
		return DomainResult{}, fmt.Errorf("payroll intent is invalid: %w", err)
	}
	if intent.Type() != intents.TypePayroll || intent.Digest() == "" || intent.Status() == intents.StatusDraft {
		return DomainResult{}, fmt.Errorf("payroll verifier requires a frozen PAYROLL intent")
	}
	if err := v.validatePlan(intent, plan); err != nil {
		return DomainResult{}, fmt.Errorf("payroll plan binding is invalid: %w", err)
	}
	result := DomainResult{Status: DomainUnverified, ReasonCode: "RECEIPT_NOT_SUCCESS", TransactionHash: strings.ToLower(strings.TrimSpace(receipt.TransactionHash))}
	fail := func(code string) (DomainResult, error) {
		result.Status = DomainFailed
		result.ReasonCode = code
		result.definitiveFailure = true
		return result, nil
	}
	if receipt.Status == providers.ReceiptReverted {
		return fail("ONCHAIN_EXECUTION_REVERTED")
	}
	if receipt.Status != providers.ReceiptSuccess {
		return result, nil
	}
	if receipt.ChainID != contracts.ChainIDArcMainnet {
		return fail("RECEIPT_CHAIN_MISMATCH")
	}
	if !providers.ValidTransactionHash(result.TransactionHash) {
		return fail("RECEIPT_HASH_INVALID")
	}
	if !receipt.HasTransaction {
		return result, nil
	}
	if !contracts.AddressesEqual(receipt.From, plan.WalletAddress()) {
		return fail("TRANSACTION_SENDER_MISMATCH")
	}
	if !contracts.AddressesEqual(receipt.To, contracts.AddressWizPayPayroll) {
		return fail("TRANSACTION_TARGET_MISMATCH")
	}
	value, err := hexutil.DecodeBig(receipt.Value)
	if err != nil || value.Sign() != 0 {
		return fail("TRANSACTION_VALUE_NONZERO")
	}
	if !bytes.Equal(receipt.Input, plan.EncodedCall().CallData()) {
		return fail("TRANSACTION_CALLDATA_PLAN_MISMATCH")
	}
	decoded, err := contractpayroll.DecodeSameTokenPayrollCall(receipt.Input)
	if err != nil {
		return fail("TRANSACTION_CALLDATA_INVALID")
	}
	financial := intent.Financial().Payroll
	if err := financial.ValidateSameToken(); err != nil {
		return fail("INTENT_NOT_SAME_TOKEN")
	}
	if !contracts.AddressesEqual(decoded.Token, financial.TokenIn.Address) || decoded.ReferenceID != financial.ReferenceID || len(decoded.Recipients) != len(financial.Recipients) || len(decoded.Amounts) != len(financial.Recipients) {
		return fail("TRANSACTION_FINANCIAL_MISMATCH")
	}
	total := new(big.Int)
	for i, recipient := range financial.Recipients {
		amount, _ := recipient.AmountIn.BaseInt()
		if !contracts.AddressesEqual(decoded.Recipients[i], recipient.Address) || decoded.Amounts[i].Cmp(amount) != 0 {
			return fail("TRANSACTION_FINANCIAL_MISMATCH")
		}
		total.Add(total, amount)
	}
	if err := providers.ValidateLogs(receipt.Logs); err != nil {
		return fail("RECEIPT_LOGS_MALFORMED")
	}
	paymentTopic, _ := contractpayroll.EventTopic0(contractpayroll.SigPayrollPayment)
	referenceTopic, _ := contractpayroll.EventTopic0(contractpayroll.SigPayrollReferenceConsumed)
	batchTopic, _ := contractpayroll.EventTopic0(contractpayroll.SigPayrollBatchExecuted)
	swapTopic, _ := contractpayroll.EventTopic0(contractpayroll.SigPayrollSwapExecuted)
	surplusTopic, _ := contractpayroll.EventTopic0(contractpayroll.SigPayrollSurplusRefunded)
	payments := make([]contractpayroll.PaymentEvent, 0, len(financial.Recipients))
	var references []contractpayroll.ReferenceConsumedEvent
	var batches []contractpayroll.BatchExecutedEvent
	for _, raw := range receipt.Logs {
		if !contracts.AddressesEqual(raw.Address, contracts.AddressWizPayPayroll) || len(raw.Topics) == 0 {
			continue
		}
		log := raw.ContractLog(receipt.ChainID)
		switch {
		case bytes.Equal(raw.Topics[0], paymentTopic):
			e, eerr := contractpayroll.DecodePaymentEvent(v.registry, log)
			if eerr != nil {
				return fail("PAYROLL_PAYMENT_MALFORMED")
			}
			payments = append(payments, e)
		case bytes.Equal(raw.Topics[0], referenceTopic):
			e, eerr := contractpayroll.DecodeReferenceConsumedEvent(v.registry, log)
			if eerr != nil {
				return fail("PAYROLL_REFERENCE_MALFORMED")
			}
			references = append(references, e)
		case bytes.Equal(raw.Topics[0], batchTopic):
			e, eerr := contractpayroll.DecodeBatchExecutedEvent(v.registry, log)
			if eerr != nil {
				return fail("PAYROLL_BATCH_MALFORMED")
			}
			batches = append(batches, e)
		case bytes.Equal(raw.Topics[0], swapTopic), bytes.Equal(raw.Topics[0], surplusTopic):
			return fail("CROSS_TOKEN_EVENT_PRESENT")
		}
	}
	if len(payments) != len(financial.Recipients) {
		return fail("PAYROLL_PAYMENT_COUNT_MISMATCH")
	}
	if len(references) != 1 || len(batches) != 1 {
		return fail("PAYROLL_AGGREGATE_EVENT_COUNT_MISMATCH")
	}
	var referenceHash [32]byte
	seen := make([]bool, len(payments))
	for position, payment := range payments {
		if payment.PaymentIndex.Sign() < 0 || !payment.PaymentIndex.IsUint64() || payment.PaymentIndex.Uint64() != uint64(position) {
			return fail("PAYROLL_PAYMENT_INDEX_MISMATCH")
		}
		i := int(payment.PaymentIndex.Uint64())
		if seen[i] {
			return fail("PAYROLL_PAYMENT_INDEX_MISMATCH")
		}
		seen[i] = true
		amount, _ := financial.Recipients[i].AmountIn.BaseInt()
		if !contracts.AddressesEqual(payment.Employer, plan.WalletAddress()) || !contracts.AddressesEqual(payment.TokenOut, financial.TokenIn.Address) || !contracts.AddressesEqual(payment.Recipient, financial.Recipients[i].Address) || payment.AmountOut.Cmp(amount) != 0 {
			return fail("PAYROLL_PAYMENT_MISMATCH")
		}
		if position == 0 {
			referenceHash = payment.ReferenceHash
		} else if payment.ReferenceHash != referenceHash {
			return fail("PAYROLL_REFERENCE_HASH_MISMATCH")
		}
	}
	ref, batch := references[0], batches[0]
	if ref.ReferenceHash != referenceHash || !contracts.AddressesEqual(ref.Employer, plan.WalletAddress()) || !contracts.AddressesEqual(ref.TokenIn, financial.TokenIn.Address) || !contracts.AddressesEqual(ref.TokenOut, financial.TokenIn.Address) || ref.ReferenceID != financial.ReferenceID {
		return fail("PAYROLL_REFERENCE_MISMATCH")
	}
	if !contracts.AddressesEqual(batch.Employer, plan.WalletAddress()) || !contracts.AddressesEqual(batch.TokenIn, financial.TokenIn.Address) || !contracts.AddressesEqual(batch.TokenOut, financial.TokenIn.Address) || batch.ReferenceID != financial.ReferenceID {
		return fail("PAYROLL_BATCH_MISMATCH")
	}
	count := big.NewInt(int64(len(financial.Recipients)))
	maximumFee := new(big.Int).Mul(new(big.Int).Set(total), new(big.Int).SetUint64(contracts.ContractMaxFeeBPS))
	maximumFee.Quo(maximumFee, big.NewInt(10_000))
	if ref.RecipientCount.Cmp(count) != 0 || batch.RecipientCount.Cmp(count) != 0 || ref.TotalOutput.Cmp(total) != 0 || batch.TotalOutput.Cmp(total) != 0 || ref.TotalInput.Cmp(batch.TotalInput) != 0 || ref.TotalFees.Cmp(batch.TotalFees) != 0 || ref.TotalFees.Cmp(maximumFee) > 0 || ref.TotalInput.Cmp(new(big.Int).Add(total, ref.TotalFees)) != 0 {
		return fail("PAYROLL_AGGREGATE_TOTAL_MISMATCH")
	}
	result.Status = DomainVerified
	result.ReasonCode = "SAME_TOKEN_PAYROLL_VERIFIED"
	result.EventSignature = contractpayroll.SigPayrollBatchExecuted
	result.Provable = []string{"sender", "target", "zero_native_value", "calldata", "ordered_payments", "aggregate_totals", "reference"}
	result.Unprovable = nil
	return result, nil
}

func (v Verifier) validatePlan(intent intents.Intent, plan Plan) error {
	if plan.IntentID() != intent.IntentID() || plan.IntentDigest() != intent.Digest() || plan.Capability() != intents.TypePayroll {
		return fmt.Errorf("plan does not bind the frozen payroll intent")
	}
	if plan.ContractID() != contracts.ContractWizPayPayroll || plan.RegistryVersion() != contracts.RegistryVersion || plan.ChainID() != contracts.ChainIDArcMainnet {
		return fmt.Errorf("plan does not bind the canonical payroll deployment")
	}
	if plan.ChainID() != intent.Ownership().ChainID || !contracts.AddressesEqual(plan.WalletAddress(), intent.Ownership().WalletAddress) {
		return fmt.Errorf("plan sender or chain does not match intent ownership")
	}
	call := plan.EncodedCall()
	if !contracts.AddressesEqual(call.To(), contracts.AddressWizPayPayroll) || call.ContractID() != contracts.ContractWizPayPayroll {
		return fmt.Errorf("plan target is not the canonical payroll contract")
	}
	deployment, err := contractpayroll.ExpectedDeployment(v.registry)
	if err != nil {
		return err
	}
	if !contracts.AddressesEqual(deployment.Address, call.To()) {
		return fmt.Errorf("plan target does not match registry")
	}
	financial := intent.Financial().Payroll
	if financial == nil || financial.ValidateSameToken() != nil {
		return fmt.Errorf("intent is not same-token payroll")
	}
	decoded, err := contractpayroll.DecodeSameTokenPayrollCall(call.CallData())
	if err != nil {
		return err
	}
	if !contracts.AddressesEqual(decoded.Token, financial.TokenIn.Address) || decoded.ReferenceID != financial.ReferenceID || len(decoded.Recipients) != len(financial.Recipients) {
		return fmt.Errorf("plan calldata does not bind intent")
	}
	for i, recipient := range financial.Recipients {
		amount, _ := recipient.AmountIn.BaseInt()
		if !contracts.AddressesEqual(decoded.Recipients[i], recipient.Address) || decoded.Amounts[i].Cmp(amount) != 0 {
			return fmt.Errorf("plan calldata does not bind intent")
		}
	}
	return nil
}
