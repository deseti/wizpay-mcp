package payroll

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractpayroll "github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

// Domain verification status for Payroll financial completion.
//
// Generic chain receipt SUCCESS is never sufficient. Only DomainVerified means
// canonical on-chain event evidence matched the immutable intent/plan for a
// capability that can fully prove settlement (SINGLE PaymentRouted).
type DomainStatus string

const (
	// DomainVerified means canonical event evidence fully matched the intent
	// for financial completion (SINGLE PaymentRouted only today).
	DomainVerified DomainStatus = "DOMAIN_VERIFIED"
	// DomainAggregateOnly means BatchPaymentRouted aggregate fields matched, but
	// recipient-level settlement cannot be proven from the event schema. This
	// is never full financial completion.
	DomainAggregateOnly DomainStatus = "DOMAIN_AGGREGATE_ONLY"
	// DomainUnverified means no usable matching event evidence was found
	// (missing logs, wrong contract, malformed, zero matches). Not terminal.
	DomainUnverified DomainStatus = "DOMAIN_UNVERIFIED"
	// DomainFailed means evidence was present but inconsistent with the frozen
	// intent/plan (wrong fields, conflicting duplicates, receipt failure).
	DomainFailed DomainStatus = "DOMAIN_FAILED"
)

// DomainResult is the pure deterministic outcome of Payroll event verification.
// It performs no RPC, wall-clock, or Circle calls.
type DomainResult struct {
	Status         DomainStatus
	ReasonCode     string
	EventSignature string
	// Provable lists fields proven from the canonical on-chain event.
	Provable []string
	// Unprovable lists intent fields the event schema cannot prove.
	// Non-empty for batch variants even when aggregate fields match.
	Unprovable []string
	// TransactionHash echoes the evidence hash that was examined.
	TransactionHash   string
	definitiveFailure bool
}

// DefinitiveFailure reports whether a DOMAIN_FAILED result is grounded in a
// canonical event whose immutable financial fields contradict the intent.
// Domain-owned orchestration and material failures deliberately return false.
func (r DomainResult) DefinitiveFailure() bool {
	return r.Status == DomainFailed && r.definitiveFailure
}

// FinancialComplete reports whether the result authorizes Payroll financial
// completion. Aggregate-only batch matches are intentionally false.
func (r DomainResult) FinancialComplete() bool {
	return r.Status == DomainVerified
}

// Verifier performs pure deterministic Payroll domain verification against
// trusted static contract event definitions and immutable intent/plan material.
//
// It never performs RPC, never accepts arbitrary event signatures, and never
// treats generic receipt success as financial success.
type Verifier struct {
	registry *contracts.Registry
}

// NewVerifier binds verification to a trusted static registry. A nil registry
// uses the repository's verified default deployment registry.
func NewVerifier(registry *contracts.Registry) Verifier {
	return Verifier{registry: registry}
}

// Verify checks chain receipt evidence against the frozen Payroll intent and
// sealed plan. All inputs must already be trusted/immutable material.
//
// Requirements for DOMAIN_VERIFIED (SINGLE only):
//   - receipt status SUCCESS
//   - plan binds the same intent digest and canonical payroll contract
//   - exactly one PaymentRouted event from the canonical payroll address
//   - sender, recipient, tokenIn, tokenOut, amountIn exact
//   - amountOut >= frozen min_amount_out (integer base units)
//
// BATCH variants at best reach DOMAIN_AGGREGATE_ONLY: BatchPaymentRouted does
// not emit per-recipient settlement detail, so recipient-level completion is
// never claimed.
func (v Verifier) Verify(intent intents.Intent, plan Plan, receipt providers.Receipt) (DomainResult, error) {
	if err := intent.Validate(); err != nil {
		return DomainResult{}, fmt.Errorf("payroll intent is invalid: %w", err)
	}
	if intent.Type() != intents.TypePayroll {
		return DomainResult{}, fmt.Errorf("payroll verifier requires PAYROLL intent")
	}
	if intent.Digest() == "" || intent.Status() == intents.StatusDraft {
		return DomainResult{}, fmt.Errorf("payroll verifier requires a frozen intent")
	}
	financial := intent.Financial().Payroll
	if financial == nil || !financial.Phase12Executable() {
		return DomainResult{}, fmt.Errorf("payroll intent is not Phase 12 executable")
	}
	if err := v.validatePlan(intent, plan); err != nil {
		return failed("PAYROLL_PLAN_MISMATCH", receipt.TransactionHash, err.Error()), nil
	}
	base := DomainResult{TransactionHash: strings.ToLower(strings.TrimSpace(receipt.TransactionHash))}

	if receipt.Status == providers.ReceiptReverted {
		base.Status = DomainFailed
		base.ReasonCode = "ONCHAIN_EXECUTION_REVERTED"
		return base, nil
	}
	if receipt.Status != providers.ReceiptSuccess {
		base.Status = DomainUnverified
		base.ReasonCode = "RECEIPT_NOT_SUCCESS"
		return base, nil
	}
	if receipt.ChainID != "" && receipt.ChainID != plan.ChainID() {
		base.Status = DomainFailed
		base.ReasonCode = "RECEIPT_CHAIN_MISMATCH"
		return base, nil
	}
	if receipt.TransactionHash != "" && plan.IntentDigest() != "" {
		// Transaction hash identity is required when present on evidence.
		if !providers.ValidTransactionHash(strings.ToLower(receipt.TransactionHash)) {
			base.Status = DomainFailed
			base.ReasonCode = "RECEIPT_HASH_INVALID"
			return base, nil
		}
	}
	if err := providers.ValidateLogs(receipt.Logs); err != nil {
		base.Status = DomainUnverified
		base.ReasonCode = "RECEIPT_LOGS_MALFORMED"
		return base, nil
	}

	switch financial.Variant {
	case intents.PayrollVariantSingle:
		return v.verifySingle(*financial, intent.Ownership(), receipt)
	case intents.PayrollVariantBatchSingleTokenOut, intents.PayrollVariantBatchMultiTokenOut:
		return v.verifyBatch(*financial, intent.Ownership(), receipt)
	default:
		base.Status = DomainFailed
		base.ReasonCode = "PAYROLL_VARIANT_UNSUPPORTED"
		return base, nil
	}
}

func (v Verifier) validatePlan(intent intents.Intent, plan Plan) error {
	if plan.IntentID() != intent.IntentID() {
		return fmt.Errorf("plan intent id mismatch")
	}
	if plan.IntentDigest() != intent.Digest() {
		return fmt.Errorf("plan intent digest mismatch")
	}
	if plan.Capability() != intents.TypePayroll {
		return fmt.Errorf("plan capability is not PAYROLL")
	}
	if plan.ContractID() != contracts.ContractWizPayPayroll {
		return fmt.Errorf("plan contract is not WIZPAY_PAYROLL")
	}
	if plan.ChainID() != intent.Ownership().ChainID {
		return fmt.Errorf("plan chain mismatch")
	}
	if !contracts.AddressesEqual(plan.WalletAddress(), intent.Ownership().WalletAddress) {
		return fmt.Errorf("plan wallet mismatch")
	}
	call := plan.EncodedCall()
	if call.To() == "" || !contracts.AddressesEqual(call.To(), contracts.AddressWizPayPayroll) {
		return fmt.Errorf("plan target is not canonical payroll")
	}
	deployment, err := contractpayroll.ExpectedDeployment(v.registry)
	if err != nil {
		return err
	}
	if !contracts.AddressesEqual(deployment.Address, contracts.AddressWizPayPayroll) {
		return fmt.Errorf("registry payroll address is not canonical")
	}
	return nil
}

func (v Verifier) verifySingle(financial intents.PayrollParameters, ownership intents.Ownership, receipt providers.Receipt) (DomainResult, error) {
	base := DomainResult{
		TransactionHash: strings.ToLower(strings.TrimSpace(receipt.TransactionHash)),
		EventSignature:  contractpayroll.SigPaymentRouted,
	}
	matches, ambiguous, err := v.collectPaymentRouted(receipt)
	if err != nil {
		base.Status = DomainUnverified
		base.ReasonCode = "PAYMENT_ROUTED_DECODE_ERROR"
		return base, nil
	}
	if ambiguous {
		base.Status = DomainUnverified
		base.ReasonCode = "PAYMENT_ROUTED_DECODE_ERROR"
		return base, nil
	}
	if len(matches) == 0 {
		base.Status = DomainUnverified
		base.ReasonCode = "PAYMENT_ROUTED_NOT_FOUND"
		return base, nil
	}
	if len(matches) > 1 {
		// Multiple identical or conflicting PaymentRouted events: fail closed.
		return definitive(base, "PAYMENT_ROUTED_AMBIGUOUS"), nil
	}
	event := matches[0]
	line := financial.Recipients[0]
	amountIn, err := line.AmountIn.BaseInt()
	if err != nil {
		return DomainResult{}, fmt.Errorf("amount_in: %w", err)
	}
	minOut, err := line.MinAmountOut.BaseInt()
	if err != nil {
		return DomainResult{}, fmt.Errorf("min_amount_out: %w", err)
	}

	if !contracts.AddressesEqual(event.Sender, ownership.WalletAddress) {
		return definitive(base, "PAYMENT_ROUTED_SENDER_MISMATCH"), nil
	}
	if !contracts.AddressesEqual(event.Recipient, line.Address) {
		return definitive(base, "PAYMENT_ROUTED_RECIPIENT_MISMATCH"), nil
	}
	if !contracts.AddressesEqual(event.TokenIn, financial.TokenIn.Address) {
		return definitive(base, "PAYMENT_ROUTED_TOKEN_IN_MISMATCH"), nil
	}
	if !contracts.AddressesEqual(event.TokenOut, line.TokenOut.Address) {
		return definitive(base, "PAYMENT_ROUTED_TOKEN_OUT_MISMATCH"), nil
	}
	if event.AmountIn == nil || event.AmountIn.Cmp(amountIn) != 0 {
		return definitive(base, "PAYMENT_ROUTED_AMOUNT_IN_MISMATCH"), nil
	}
	if event.AmountOut == nil || event.AmountOut.Sign() < 0 {
		return definitive(base, "PAYMENT_ROUTED_AMOUNT_OUT_INVALID"), nil
	}
	// Settlement proof: actual output must meet the frozen minimum.
	if event.AmountOut.Cmp(minOut) < 0 {
		return definitive(base, "PAYMENT_ROUTED_AMOUNT_OUT_BELOW_MINIMUM"), nil
	}
	if event.FeeAmount == nil || event.FeeAmount.Sign() < 0 {
		return definitive(base, "PAYMENT_ROUTED_FEE_INVALID"), nil
	}

	base.Status = DomainVerified
	base.ReasonCode = ""
	base.Provable = []string{
		"emitting_address", "event_signature", "sender", "recipient",
		"token_in", "token_out", "amount_in", "amount_out_gte_min", "fee_amount_non_negative",
	}
	return base, nil
}

func (v Verifier) verifyBatch(financial intents.PayrollParameters, ownership intents.Ownership, receipt providers.Receipt) (DomainResult, error) {
	base := DomainResult{
		TransactionHash: strings.ToLower(strings.TrimSpace(receipt.TransactionHash)),
		EventSignature:  contractpayroll.SigBatchPaymentRouted,
		// Explicit recipient-level evidence limitations of BatchPaymentRouted.
		Unprovable: batchUnprovableFields(financial.Variant),
	}
	matches, ambiguous, err := v.collectBatchPaymentRouted(receipt)
	if err != nil {
		base.Status = DomainUnverified
		base.ReasonCode = "BATCH_PAYMENT_ROUTED_DECODE_ERROR"
		return base, nil
	}
	if ambiguous {
		base.Status = DomainUnverified
		base.ReasonCode = "BATCH_PAYMENT_ROUTED_DECODE_ERROR"
		return base, nil
	}
	if len(matches) == 0 {
		base.Status = DomainUnverified
		base.ReasonCode = "BATCH_PAYMENT_ROUTED_NOT_FOUND"
		return base, nil
	}
	if len(matches) > 1 {
		return definitive(base, "BATCH_PAYMENT_ROUTED_AMBIGUOUS"), nil
	}
	event := matches[0]

	if !contracts.AddressesEqual(event.Sender, ownership.WalletAddress) {
		return definitive(base, "BATCH_PAYMENT_ROUTED_SENDER_MISMATCH"), nil
	}
	if event.ReferenceID != financial.ReferenceID {
		return definitive(base, "BATCH_PAYMENT_ROUTED_REFERENCE_MISMATCH"), nil
	}
	if !contracts.AddressesEqual(event.TokenIn, financial.TokenIn.Address) {
		return definitive(base, "BATCH_PAYMENT_ROUTED_TOKEN_IN_MISMATCH"), nil
	}
	totalIn, err := financial.Total.BaseInt()
	if err != nil {
		return DomainResult{}, fmt.Errorf("total: %w", err)
	}
	if event.TotalAmountIn == nil || event.TotalAmountIn.Cmp(totalIn) != 0 {
		return definitive(base, "BATCH_PAYMENT_ROUTED_TOTAL_IN_MISMATCH"), nil
	}
	wantCount := big.NewInt(int64(len(financial.Recipients)))
	if event.RecipientCount == nil || event.RecipientCount.Cmp(wantCount) != 0 {
		return definitive(base, "BATCH_PAYMENT_ROUTED_RECIPIENT_COUNT_MISMATCH"), nil
	}
	if event.TotalAmountOut == nil || event.TotalAmountOut.Sign() < 0 {
		return definitive(base, "BATCH_PAYMENT_ROUTED_TOTAL_OUT_INVALID"), nil
	}
	if event.TotalFees == nil || event.TotalFees.Sign() < 0 {
		return definitive(base, "BATCH_PAYMENT_ROUTED_FEES_INVALID"), nil
	}

	// Sum of frozen min_amount_out is a lower bound on aggregate settlement.
	minTotalOut := new(big.Int)
	for i, line := range financial.Recipients {
		minOut, err := line.MinAmountOut.BaseInt()
		if err != nil {
			return DomainResult{}, fmt.Errorf("recipients[%d].min_amount_out: %w", i, err)
		}
		minTotalOut.Add(minTotalOut, minOut)
	}
	if event.TotalAmountOut.Cmp(minTotalOut) < 0 {
		return definitive(base, "BATCH_PAYMENT_ROUTED_TOTAL_OUT_BELOW_MINIMUM"), nil
	}

	provable := []string{
		"emitting_address", "event_signature", "sender", "token_in",
		"total_amount_in", "total_amount_out_gte_sum_min", "total_fees_non_negative",
		"recipient_count", "reference_id",
	}

	switch financial.Variant {
	case intents.PayrollVariantBatchSingleTokenOut:
		// Single token-out: event tokenOut must match the shared token_out.
		tokenOut := financial.Recipients[0].TokenOut.Address
		if !contracts.AddressesEqual(event.TokenOut, tokenOut) {
			return definitive(base, "BATCH_PAYMENT_ROUTED_TOKEN_OUT_MISMATCH"), nil
		}
		provable = append(provable, "token_out")
	case intents.PayrollVariantBatchMultiTokenOut:
		// Event schema exposes a single tokenOut; multi-token lines cannot be
		// proven. Do not claim token_out match.
		// Still require a non-zero address so the log is well-formed.
		if !contracts.ValidAddress(event.TokenOut) {
			return definitive(base, "BATCH_PAYMENT_ROUTED_TOKEN_OUT_INVALID"), nil
		}
	}

	// Aggregate fields matched, but recipient-level settlement is unprovable.
	// Never claim DomainVerified / financial completion for batch.
	base.Status = DomainAggregateOnly
	base.ReasonCode = "BATCH_RECIPIENT_LEVEL_UNPROVEN"
	base.Provable = provable
	return base, nil
}

func failed(code, txHash, _ string) DomainResult {
	return DomainResult{
		Status:          DomainFailed,
		ReasonCode:      code,
		TransactionHash: strings.ToLower(strings.TrimSpace(txHash)),
	}
}

func definitive(result DomainResult, code string) DomainResult {
	result.Status = DomainFailed
	result.ReasonCode = code
	result.definitiveFailure = true
	return result
}
