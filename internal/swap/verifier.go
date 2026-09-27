package swap

import (
	"fmt"
	"strings"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractswap "github.com/deseti/wizpay-mcp/internal/contracts/swap"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

type DomainStatus string

const (
	DomainVerified   DomainStatus = "DOMAIN_VERIFIED"
	DomainUnverified DomainStatus = "DOMAIN_UNVERIFIED"
	DomainFailed     DomainStatus = "DOMAIN_FAILED"
)

type DomainResult struct {
	Status            DomainStatus
	ReasonCode        string
	EventSignature    string
	Provable          []string
	TransactionHash   string
	definitiveFailure bool
}

func (r DomainResult) DefinitiveFailure() bool {
	return r.Status == DomainFailed && r.definitiveFailure
}
func (r DomainResult) FinancialComplete() bool { return r.Status == DomainVerified }

// Verifier remains fail-closed until a later track implements and reviews the
// Mainnet swap execution and receipt semantics.
type Verifier struct{ registry *contracts.Registry }

func NewVerifier(registry *contracts.Registry) Verifier { return Verifier{registry: registry} }

func (v Verifier) Verify(intent intents.Intent, plan Plan, receipt providers.Receipt) (DomainResult, error) {
	if err := intent.Validate(); err != nil {
		return DomainResult{}, fmt.Errorf("swap intent is invalid: %w", err)
	}
	if intent.Type() != intents.TypeSwap || intent.Digest() == "" || intent.Status() == intents.StatusDraft {
		return DomainResult{}, fmt.Errorf("swap verifier requires a frozen SWAP intent")
	}
	if err := v.validatePlan(intent, plan); err != nil {
		return DomainResult{}, fmt.Errorf("swap plan binding is invalid: %w", err)
	}
	result := DomainResult{
		Status:          DomainUnverified,
		ReasonCode:      "ARC_MAINNET_SWAP_EXECUTION_DISABLED",
		TransactionHash: strings.ToLower(strings.TrimSpace(receipt.TransactionHash)),
		Provable:        nil,
	}
	if receipt.Status == providers.ReceiptReverted {
		result.Status = DomainFailed
		result.ReasonCode = "ONCHAIN_EXECUTION_REVERTED"
		return result, nil
	}
	if receipt.Status != providers.ReceiptSuccess {
		result.ReasonCode = "RECEIPT_NOT_SUCCESS"
		return result, nil
	}
	if receipt.ChainID != "" && receipt.ChainID != plan.ChainID() {
		result.Status = DomainFailed
		result.ReasonCode = "RECEIPT_CHAIN_MISMATCH"
		return result, nil
	}
	if receipt.TransactionHash != "" && !providers.ValidTransactionHash(result.TransactionHash) {
		result.Status = DomainFailed
		result.ReasonCode = "RECEIPT_HASH_INVALID"
		return result, nil
	}
	if err := providers.ValidateLogs(receipt.Logs); err != nil {
		result.ReasonCode = "RECEIPT_LOGS_MALFORMED"
	}
	return result, nil
}

func (v Verifier) validatePlan(intent intents.Intent, plan Plan) error {
	if plan.IntentID() != intent.IntentID() || plan.IntentDigest() != intent.Digest() || plan.Capability() != intents.TypeSwap {
		return fmt.Errorf("plan does not bind the frozen swap intent")
	}
	if plan.ContractID() != contracts.ContractWizPaySwapExecutor || plan.RegistryVersion() != contracts.RegistryVersion || plan.ChainID() != contracts.ChainIDArcMainnet {
		return fmt.Errorf("plan does not bind the canonical swap deployment")
	}
	if plan.ChainID() != intent.Ownership().ChainID || !contracts.AddressesEqual(plan.WalletAddress(), intent.Ownership().WalletAddress) {
		return fmt.Errorf("plan sender or chain does not match intent ownership")
	}
	call := plan.EncodedCall()
	if !contracts.AddressesEqual(call.To(), contracts.AddressWizPaySwapExecutor) || call.ContractID() != contracts.ContractWizPaySwapExecutor {
		return fmt.Errorf("plan target is not the canonical swap contract")
	}
	deployment, err := contractswap.ExpectedDeployment(v.registry)
	if err != nil {
		return err
	}
	if !contracts.AddressesEqual(deployment.Address, call.To()) {
		return fmt.Errorf("plan target does not match registry")
	}
	return nil
}
