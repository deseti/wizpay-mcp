package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/execution/runtime"
)

// ReceiptStatus is the provider-neutral on-chain outcome of one transaction.
type ReceiptStatus string

const (
	// ReceiptUnknown means the chain has no receipt yet. It is not a failure:
	// an unmined or unseen transaction may still confirm.
	ReceiptUnknown ReceiptStatus = "UNKNOWN"
	// ReceiptSuccess means a receipt confirmed successful execution.
	ReceiptSuccess ReceiptStatus = "SUCCESS"
	// ReceiptReverted means a receipt confirmed the transaction executed and
	// failed. This is the only evidence permitted to fail an execution.
	ReceiptReverted ReceiptStatus = "REVERTED"
	// ReceiptReorgInconsistent means successive observations of the same
	// transaction disagree (missing after present, block hash/number change, or
	// confirmation depth decrease). It is reconciliation-only and never success.
	ReceiptReorgInconsistent ReceiptStatus = "REORG_INCONSISTENT"
)

// Receipt is the minimal on-chain evidence needed to verify an execution. It
// carries no logs, no raw response, and no decoded contract payload.
type Receipt struct {
	Status          ReceiptStatus
	ChainID         string
	TransactionHash string
	BlockNumber     uint64
	// BlockHash is the inclusion block hash when the chain reports one. It is
	// used for reorg detection across verification passes.
	BlockHash     string
	Confirmations uint64
}

// ChainVerifier reads transaction receipts from a chain. Implementations must
// expose this narrow surface only: never arbitrary JSON-RPC, never arbitrary
// contract execution, and never signing.
type ChainVerifier interface {
	TransactionReceipt(ctx context.Context, chainID, transactionHash string) (Receipt, error)
}

// ReferenceResolver enriches a known provider reference with the on-chain
// transaction hash once the provider reports one. It performs read-only
// reconciliation and must never submit or resubmit anything.
//
// The execution ID is passed so the implementation can locate the ephemeral
// user authorization scoped to that execution; reconciliation reads are
// user-scoped at the provider.
type ReferenceResolver interface {
	ResolveReference(ctx context.Context, executionID string, reference Reference) (Reference, error)
}

// VerifierConfig bounds when on-chain evidence is accepted as final.
type VerifierConfig struct {
	// MinConfirmations is the confirmation depth required before a successful
	// receipt is treated as verified. It must be at least 1 so that a receipt
	// from an unconfirmed block can never verify an execution.
	MinConfirmations uint64
}

func (c VerifierConfig) Validate() error {
	if c.MinConfirmations < 1 {
		return fmt.Errorf("verifier requires at least one confirmation")
	}
	return nil
}

// Verifier implements the Phase 9 runtime.Verifier boundary by combining
// provider reconciliation with on-chain receipt verification.
//
// The rule it enforces is the reason this type exists: a provider status is
// never sufficient for VERIFIED. Only a chain receipt, on the chain the
// reference names, at the required confirmation depth, can verify an
// execution, and only a receipt can fail one.
//
// Reorg-aware observation tracking compares successive receipts for the same
// transaction. Inconsistency yields a non-success, reconciliation-only outcome
// and never triggers resubmission.
type Verifier struct {
	chain        ChainVerifier
	resolver     ReferenceResolver
	config       VerifierConfig
	now          func() time.Time
	observations *ObservationTracker
}

func NewVerifier(chain ChainVerifier, resolver ReferenceResolver, config VerifierConfig, now func() time.Time) (*Verifier, error) {
	if chain == nil || resolver == nil || now == nil {
		return nil, fmt.Errorf("provider verifier dependencies are required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Verifier{
		chain:        chain,
		resolver:     resolver,
		config:       config,
		now:          now,
		observations: NewObservationTracker(),
	}, nil
}

// Verify resolves the persisted provider reference to on-chain evidence. The
// execution supplies only its identity: the reference alone names the
// transaction, and the runtime has already scoped it to this execution.
func (v *Verifier) Verify(ctx context.Context, value execution.Execution, encoded string) (runtime.VerificationResult, error) {
	// A reference this layer cannot parse is never treated as absence of a
	// transaction. It is surfaced as ambiguous so the runtime holds the
	// execution for recovery instead of assuming an outcome.
	reference, err := ParseReference(encoded)
	if err != nil {
		return runtime.VerificationResult{}, err
	}
	reference, err = v.resolveHash(ctx, value.ExecutionID(), reference)
	if err != nil {
		return runtime.VerificationResult{}, err
	}
	if reference.TransactionHash == "" {
		// The provider has not yet reported an on-chain transaction. Pending
		// preserves whatever reference material is already known.
		return v.pending(reference)
	}
	receipt, err := v.chain.TransactionReceipt(ctx, reference.ChainID, reference.TransactionHash)
	if err != nil {
		// Chain unavailability is not evidence of failure.
		return runtime.VerificationResult{}, runtime.NewVerificationError("CHAIN_VERIFICATION_UNAVAILABLE")
	}
	if err := v.ensureReceiptMatches(reference, receipt); err != nil {
		return runtime.VerificationResult{}, err
	}

	// Reorg/inconsistency detection compares this observation to the previous
	// one for the same transaction. A reorg signal is reconciliation-only:
	// never verified success, never automatic resubmission.
	//
	// A receipt is "present" when the chain returned inclusion metadata or a
	// terminal status. A completely absent receipt has neither.
	present := receipt.BlockNumber != 0 || receipt.BlockHash != "" ||
		receipt.Status == ReceiptSuccess || receipt.Status == ReceiptReverted
	signal := v.observations.Evaluate(ObservationFromReceipt(receipt, present))
	if signal.Inconsistent() || receipt.Status == ReceiptReorgInconsistent {
		// Stay pending so the runtime continues reconciling the same execution.
		// Verification PENDING never completes and never resubmits.
		return v.pending(reference)
	}

	outcome := Outcome{Reference: reference, HasReference: true, ObservedAt: v.now().UTC()}
	switch receipt.Status {
	case ReceiptSuccess:
		if receipt.Confirmations < v.config.MinConfirmations {
			return v.pending(reference)
		}
		outcome.Class = ClassVerifiedSuccess
	case ReceiptReverted:
		outcome.Class = ClassConfirmedOnchainFailed
		outcome.ReasonCode = "ONCHAIN_EXECUTION_REVERTED"
	case ReceiptUnknown, ReceiptReorgInconsistent:
		return v.pending(reference)
	default:
		// An unrecognized receipt status fails closed to pending rather than
		// being interpreted as either success or failure.
		return v.pending(reference)
	}
	return outcome.VerificationResult()
}

// resolveHash asks the provider for the on-chain hash when one is not already
// known. A resolver failure leaves the execution pending rather than failing.
func (v *Verifier) resolveHash(ctx context.Context, executionID string, reference Reference) (Reference, error) {
	if reference.TransactionHash != "" {
		return reference, nil
	}
	resolved, err := v.resolver.ResolveReference(ctx, executionID, reference)
	if err != nil {
		return Reference{}, runtime.NewVerificationError("PROVIDER_RECONCILIATION_UNAVAILABLE")
	}
	// The resolver may only add identifiers to the reference it was given. A
	// resolver that renames the provider, switches chains, or replaces a known
	// transaction is rejected rather than trusted.
	if resolved.Provider != reference.Provider || resolved.ChainID != reference.ChainID {
		return Reference{}, fmt.Errorf("resolved provider reference changed provider identity")
	}
	if reference.ProviderTransactionID != "" && resolved.ProviderTransactionID != reference.ProviderTransactionID {
		return Reference{}, fmt.Errorf("resolved provider reference changed the provider transaction")
	}
	if err := resolved.Validate(); err != nil {
		return Reference{}, err
	}
	return resolved, nil
}

// ensureReceiptMatches rejects a receipt that does not correspond to the exact
// transaction on the exact chain the reference names.
func (v *Verifier) ensureReceiptMatches(reference Reference, receipt Receipt) error {
	if receipt.Status == ReceiptUnknown {
		return nil
	}
	if receipt.ChainID != reference.ChainID {
		return fmt.Errorf("receipt chain does not match the provider reference")
	}
	if !strings.EqualFold(receipt.TransactionHash, reference.TransactionHash) {
		return fmt.Errorf("receipt transaction does not match the provider reference")
	}
	return nil
}

func (v *Verifier) pending(reference Reference) (runtime.VerificationResult, error) {
	encoded, err := reference.Encode()
	if err != nil {
		return runtime.VerificationResult{}, err
	}
	return runtime.VerificationResult{Outcome: runtime.VerificationPending, Reference: encoded, ObservedAt: v.now().UTC()}, nil
}

var _ runtime.Verifier = (*Verifier)(nil)
