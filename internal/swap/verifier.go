package swap

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractswap "github.com/deseti/wizpay-mcp/internal/contracts/swap"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

// Domain verification status for Swap financial completion.
//
// Generic chain receipt SUCCESS is never sufficient. Only DomainVerified means
// a canonical WizPaySwapExecuted event matched the immutable intent/plan.
type DomainStatus string

const (
	// DomainVerified means canonical event evidence fully matched the intent.
	DomainVerified DomainStatus = "DOMAIN_VERIFIED"
	// DomainUnverified means no usable matching event evidence was found.
	DomainUnverified DomainStatus = "DOMAIN_UNVERIFIED"
	// DomainFailed means evidence was present but inconsistent with the intent.
	DomainFailed DomainStatus = "DOMAIN_FAILED"
)

// DomainResult is the pure deterministic outcome of Swap event verification.
// It performs no RPC, wall-clock, or Circle calls.
type DomainResult struct {
	Status          DomainStatus
	ReasonCode      string
	EventSignature  string
	Provable        []string
	TransactionHash string
}

// FinancialComplete reports whether the result authorizes Swap financial completion.
func (r DomainResult) FinancialComplete() bool {
	return r.Status == DomainVerified
}

// Verifier performs pure deterministic Swap domain verification against trusted
// static contract event definitions and immutable intent/plan material.
type Verifier struct {
	registry *contracts.Registry
}

// NewVerifier binds verification to a trusted static registry. A nil registry
// uses the repository's verified default deployment registry.
func NewVerifier(registry *contracts.Registry) Verifier {
	return Verifier{registry: registry}
}

// Verify checks chain receipt evidence against the frozen Swap intent and
// sealed plan.
//
// Requirements for DOMAIN_VERIFIED:
//   - receipt status SUCCESS
//   - plan binds the same intent digest and canonical swap executor
//   - exactly one WizPaySwapExecuted from the canonical swap address
//   - user, router, tokenIn, tokenOut, amountIn, recipient exact
//   - amountOut >= frozen MinimumOutput (not ExpectedOutput)
//   - feeAmount non-negative when present (no second MCP fee)
func (v Verifier) Verify(intent intents.Intent, plan Plan, receipt providers.Receipt) (DomainResult, error) {
	if err := intent.Validate(); err != nil {
		return DomainResult{}, fmt.Errorf("swap intent is invalid: %w", err)
	}
	if intent.Type() != intents.TypeSwap {
		return DomainResult{}, fmt.Errorf("swap verifier requires SWAP intent")
	}
	if intent.Digest() == "" || intent.Status() == intents.StatusDraft {
		return DomainResult{}, fmt.Errorf("swap verifier requires a frozen intent")
	}
	financial := intent.Financial().Swap
	if financial == nil || !financial.Phase12Executable() {
		return DomainResult{}, fmt.Errorf("swap intent is not Phase 12 executable")
	}
	if err := v.validatePlan(intent, plan); err != nil {
		return DomainResult{
			Status:          DomainFailed,
			ReasonCode:      "SWAP_PLAN_MISMATCH",
			TransactionHash: strings.ToLower(strings.TrimSpace(receipt.TransactionHash)),
		}, nil
	}
	base := DomainResult{
		TransactionHash: strings.ToLower(strings.TrimSpace(receipt.TransactionHash)),
		EventSignature:  contractswap.SigWizPaySwapExecuted,
	}

	if receipt.Status == providers.ReceiptReverted {
		base.Status = DomainFailed
		base.ReasonCode = "ONCHAIN_EXECUTION_REVERTED"
		return base, nil
	}
	if receipt.Status != providers.ReceiptSuccess {
		// Provider/chain receipt not yet SUCCESS: domain remains unverified.
		// This covers "provider receipt success without valid event" when the
		// receipt is SUCCESS but also when callers pass incomplete evidence.
		base.Status = DomainUnverified
		base.ReasonCode = "RECEIPT_NOT_SUCCESS"
		return base, nil
	}
	if receipt.ChainID != "" && receipt.ChainID != plan.ChainID() {
		base.Status = DomainFailed
		base.ReasonCode = "RECEIPT_CHAIN_MISMATCH"
		return base, nil
	}
	if receipt.TransactionHash != "" && !providers.ValidTransactionHash(strings.ToLower(receipt.TransactionHash)) {
		base.Status = DomainFailed
		base.ReasonCode = "RECEIPT_HASH_INVALID"
		return base, nil
	}
	if err := providers.ValidateLogs(receipt.Logs); err != nil {
		base.Status = DomainUnverified
		base.ReasonCode = "RECEIPT_LOGS_MALFORMED"
		return base, nil
	}

	matches, ambiguous, err := v.collectSwapExecuted(receipt)
	if err != nil {
		base.Status = DomainUnverified
		base.ReasonCode = "SWAP_EXECUTED_DECODE_ERROR"
		return base, nil
	}
	if ambiguous {
		base.Status = DomainFailed
		base.ReasonCode = "SWAP_EXECUTED_AMBIGUOUS"
		return base, nil
	}
	if len(matches) == 0 {
		// Receipt SUCCESS without a valid WizPaySwapExecuted is not domain verified.
		base.Status = DomainUnverified
		base.ReasonCode = "SWAP_EXECUTED_NOT_FOUND"
		return base, nil
	}
	if len(matches) > 1 {
		base.Status = DomainFailed
		base.ReasonCode = "SWAP_EXECUTED_AMBIGUOUS"
		return base, nil
	}
	event := matches[0]

	amountIn, err := financial.InputAmount.BaseInt()
	if err != nil {
		return DomainResult{}, fmt.Errorf("input amount: %w", err)
	}
	minOut, err := financial.MinimumOutput.BaseInt()
	if err != nil {
		return DomainResult{}, fmt.Errorf("minimum output: %w", err)
	}

	// User is the wallet that initiated executeSwap (ownership wallet).
	if !contracts.AddressesEqual(event.User, ownershipWallet(intent)) {
		base.Status = DomainFailed
		base.ReasonCode = "SWAP_EXECUTED_USER_MISMATCH"
		return base, nil
	}
	if !contracts.AddressesEqual(event.Router, financial.Router) {
		base.Status = DomainFailed
		base.ReasonCode = "SWAP_EXECUTED_ROUTER_MISMATCH"
		return base, nil
	}
	if !contracts.AddressesEqual(event.TokenIn, financial.InputToken.Address) {
		base.Status = DomainFailed
		base.ReasonCode = "SWAP_EXECUTED_TOKEN_IN_MISMATCH"
		return base, nil
	}
	if !contracts.AddressesEqual(event.TokenOut, financial.OutputToken.Address) {
		base.Status = DomainFailed
		base.ReasonCode = "SWAP_EXECUTED_TOKEN_OUT_MISMATCH"
		return base, nil
	}
	if event.AmountIn == nil || event.AmountIn.Cmp(amountIn) != 0 {
		base.Status = DomainFailed
		base.ReasonCode = "SWAP_EXECUTED_AMOUNT_IN_MISMATCH"
		return base, nil
	}
	if event.AmountOut == nil || event.AmountOut.Sign() < 0 {
		base.Status = DomainFailed
		base.ReasonCode = "SWAP_EXECUTED_AMOUNT_OUT_INVALID"
		return base, nil
	}
	// Critical: compare against MinimumOutput only. ExpectedOutput is quote info.
	if event.AmountOut.Cmp(minOut) < 0 {
		base.Status = DomainFailed
		base.ReasonCode = "SWAP_EXECUTED_AMOUNT_OUT_BELOW_MINIMUM"
		return base, nil
	}
	if !contracts.AddressesEqual(event.Recipient, financial.Recipient) {
		base.Status = DomainFailed
		base.ReasonCode = "SWAP_EXECUTED_RECIPIENT_MISMATCH"
		return base, nil
	}
	if event.FeeAmount == nil || event.FeeAmount.Sign() < 0 {
		base.Status = DomainFailed
		base.ReasonCode = "SWAP_EXECUTED_FEE_INVALID"
		return base, nil
	}
	if event.NetAmountIn == nil || event.NetAmountIn.Sign() < 0 {
		base.Status = DomainFailed
		base.ReasonCode = "SWAP_EXECUTED_NET_AMOUNT_IN_INVALID"
		return base, nil
	}
	// Fee consistency with contract semantics: feeAmount + netAmountIn == amountIn.
	// This validates observed emission only; it does not introduce an MCP fee.
	sum := new(big.Int).Add(event.FeeAmount, event.NetAmountIn)
	if sum.Cmp(event.AmountIn) != 0 {
		base.Status = DomainFailed
		base.ReasonCode = "SWAP_EXECUTED_FEE_SEMANTICS_MISMATCH"
		return base, nil
	}

	base.Status = DomainVerified
	base.ReasonCode = ""
	base.Provable = []string{
		"emitting_address", "event_signature", "user", "router",
		"token_in", "token_out", "amount_in", "amount_out_gte_minimum",
		"recipient", "fee_amount_non_negative", "net_amount_in",
	}
	return base, nil
}

func (v Verifier) validatePlan(intent intents.Intent, plan Plan) error {
	if plan.IntentID() != intent.IntentID() {
		return fmt.Errorf("plan intent id mismatch")
	}
	if plan.IntentDigest() != intent.Digest() {
		return fmt.Errorf("plan intent digest mismatch")
	}
	if plan.Capability() != intents.TypeSwap {
		return fmt.Errorf("plan capability is not SWAP")
	}
	if plan.ContractID() != contracts.ContractWizPaySwapExecutor {
		return fmt.Errorf("plan contract is not WIZPAY_SWAP_EXECUTOR")
	}
	if plan.ChainID() != intent.Ownership().ChainID {
		return fmt.Errorf("plan chain mismatch")
	}
	if !contracts.AddressesEqual(plan.WalletAddress(), intent.Ownership().WalletAddress) {
		return fmt.Errorf("plan wallet mismatch")
	}
	call := plan.EncodedCall()
	if call.To() == "" || !contracts.AddressesEqual(call.To(), contracts.AddressWizPaySwapExecutor) {
		return fmt.Errorf("plan target is not canonical swap executor")
	}
	deployment, err := contractswap.ExpectedDeployment(v.registry)
	if err != nil {
		return err
	}
	if !contracts.AddressesEqual(deployment.Address, contracts.AddressWizPaySwapExecutor) {
		return fmt.Errorf("registry swap address is not canonical")
	}
	return nil
}

func (v Verifier) collectSwapExecuted(receipt providers.Receipt) (matches []contractswap.WizPaySwapExecuted, ambiguous bool, err error) {
	var decoded []contractswap.WizPaySwapExecuted
	var malformed int
	topic0, terr := contractswap.EventTopic0()
	if terr != nil {
		return nil, false, terr
	}
	for _, log := range receipt.Logs {
		if !contracts.ValidAddress(log.Address) || !contracts.AddressesEqual(log.Address, contracts.AddressWizPaySwapExecutor) {
			continue
		}
		if len(log.Topics) == 0 {
			continue
		}
		if !topicsEqual(log.Topics[0], topic0) {
			continue
		}
		event, derr := contractswap.DecodeWizPaySwapExecuted(v.registry, log.ContractLog(receipt.ChainID))
		if derr != nil {
			malformed++
			continue
		}
		decoded = append(decoded, event)
	}
	if malformed > 0 && len(decoded) == 0 {
		return nil, false, fmt.Errorf("malformed WizPaySwapExecuted log")
	}
	if malformed > 0 && len(decoded) > 0 {
		return decoded, true, nil
	}
	return decoded, false, nil
}

func ownershipWallet(intent intents.Intent) string {
	return intent.Ownership().WalletAddress
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
