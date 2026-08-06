package providers

import (
	"fmt"
	"strings"
	"sync"
)

// ReceiptObservation is one prior on-chain receipt observation used for
// defensive observation-integrity detection. It carries no provider secrets.
//
// On Arc, committed blocks are not expected to reorg under deterministic BFT
// finality. These comparisons still protect against contradictory RPC/provider
// observations (disappearing receipts, hash/number mismatch, confirmation/head
// regression, malformed responses). Inconsistency is fail-closed and
// reconciliation-only; it never authorizes resubmission.
//
// When Present is false after a prior present observation, BlockHash/BlockNumber
// retain the last known inclusion identity so a later reappearance can still be
// compared. Confirmations are only meaningful while Present is true.
//
// LogFingerprint, when non-empty, is a deterministic hash of normalized receipt
// logs for the observed inclusion. A change under the same block identity is an
// observation-integrity failure (fail closed). Full log bodies are never stored
// here or in the adapter reference.
type ReceiptObservation struct {
	ChainID         string
	TransactionHash string
	Status          ReceiptStatus
	BlockNumber     uint64
	BlockHash       string
	Confirmations   uint64
	// Present is true when a receipt body was observed on this pass.
	Present bool
	// Known is true when this observation carries durable inclusion history
	// (present now, or retained last-known inclusion after a missing receipt).
	Known bool
	// LogFingerprint is the hex SHA-256 of normalized logs when any logs were
	// observed. Empty means no logs (or pre-log-fingerprint observations).
	LogFingerprint string
}

// HasInclusion reports whether last-known inclusion identity is available.
func (o ReceiptObservation) HasInclusion() bool {
	return o.Known || o.Present || o.BlockHash != "" || o.BlockNumber != 0
}

// ReorgSignal classifies a comparison between a prior observation and a new one.
//
// The type name is historical. On Arc these signals describe defensive
// observation/RPC integrity failures, not an expected consensus reorg path.
type ReorgSignal string

const (
	// ReorgNone means the new observation is consistent with the prior one
	// (including normal confirmation growth or first observation).
	ReorgNone ReorgSignal = "NONE"
	// ReorgReceiptMissing means a previously present receipt is no longer found
	// (treat as RPC/observation inconsistency on Arc).
	ReorgReceiptMissing ReorgSignal = "RECEIPT_MISSING"
	// ReorgBlockHashChanged means the inclusion block hash changed across
	// observations (defensive integrity signal; not expected Arc consensus reorg).
	ReorgBlockHashChanged ReorgSignal = "BLOCK_HASH_CHANGED"
	// ReorgBlockNumberChanged means the inclusion block number changed across
	// observations (defensive integrity signal).
	ReorgBlockNumberChanged ReorgSignal = "BLOCK_NUMBER_CHANGED"
	// ReorgConfirmationsDecreased means confirmation depth fell after a prior
	// present observation of the same inclusion (head/RPC regression).
	ReorgConfirmationsDecreased ReorgSignal = "CONFIRMATIONS_DECREASED"
	// ReorgLogsChanged means normalized receipt logs changed for the same
	// transaction/block inclusion identity (defensive RPC integrity signal).
	ReorgLogsChanged ReorgSignal = "LOGS_CHANGED"
)

func (s ReorgSignal) Inconsistent() bool {
	return s != ReorgNone && s != ""
}

// CompareReceiptObservations detects meaningful observation inconsistency
// between a prior observation and a newly read receipt. A first observation
// (no prior inclusion identity) always returns ReorgNone. Inconsistency never
// implies resubmission — only reconciliation.
func CompareReceiptObservations(prior, current ReceiptObservation) ReorgSignal {
	if !prior.HasInclusion() {
		return ReorgNone
	}
	if !current.Present {
		// Only signal missing when we previously observed presence. Staying
		// missing after a missing observation is not a new inconsistency.
		if prior.Present {
			return ReorgReceiptMissing
		}
		return ReorgNone
	}
	// Current is present: compare against last known inclusion identity.
	if prior.BlockHash != "" && current.BlockHash != "" &&
		!strings.EqualFold(prior.BlockHash, current.BlockHash) {
		return ReorgBlockHashChanged
	}
	if prior.BlockNumber != 0 && current.BlockNumber != 0 && prior.BlockNumber != current.BlockNumber {
		return ReorgBlockNumberChanged
	}
	// Confirmation regression only applies when both observations are present
	// for the same inclusion (hash/number already match or were empty).
	if prior.Present && prior.Confirmations > 0 && current.Confirmations < prior.Confirmations {
		return ReorgConfirmationsDecreased
	}
	// Log fingerprint mismatch under the same inclusion is fail-closed.
	// - Both non-empty and unequal → changed
	// - Prior non-empty, current empty while still present → logs disappeared
	// An empty prior (legacy observation without logs) does not signal when the
	// current observation first supplies one.
	if prior.Present && current.Present && prior.LogFingerprint != "" {
		if current.LogFingerprint == "" || prior.LogFingerprint != current.LogFingerprint {
			return ReorgLogsChanged
		}
	}
	return ReorgNone
}

// MergeObservation updates durable observation state after a compare.
//
// present -> missing: Present=false but last inclusion identity is retained.
// missing -> present: inclusion identity updates to the reappeared receipt.
// present -> present: inclusion and confirmations update to current.
func MergeObservation(prior, current ReceiptObservation) ReceiptObservation {
	out := current
	out.TransactionHash = strings.ToLower(strings.TrimSpace(current.TransactionHash))
	out.ChainID = current.ChainID
	if current.Present {
		out.Known = true
		out.Present = true
		out.BlockHash = strings.ToLower(strings.TrimSpace(current.BlockHash))
		out.BlockNumber = current.BlockNumber
		out.Confirmations = current.Confirmations
		out.Status = current.Status
		out.LogFingerprint = strings.ToLower(strings.TrimSpace(current.LogFingerprint))
		return out
	}
	// Missing now: retain last known inclusion if any.
	if prior.HasInclusion() {
		out.Known = true
		out.Present = false
		out.BlockHash = prior.BlockHash
		out.BlockNumber = prior.BlockNumber
		// Do not retain prior confirmations across absence; reappearance must
		// rebuild confirmation depth against the retained inclusion identity.
		out.Confirmations = 0
		out.Status = ReceiptUnknown
		// Retain log fingerprint with inclusion identity so a reappearance can
		// still detect log mutation against the last known receipt body.
		out.LogFingerprint = prior.LogFingerprint
		return out
	}
	out.Known = false
	out.Present = false
	return out
}

// ObservationTracker stores the latest receipt observation per transaction so
// subsequent verification passes can detect contradictory observations
// in-process. Durable recovery after restart is supplied by seeding Evaluate
// with observation metadata decoded from persisted adapter references.
type ObservationTracker struct {
	mu   sync.Mutex
	byTX map[string]ReceiptObservation
}

// NewObservationTracker constructs an empty tracker.
func NewObservationTracker() *ObservationTracker {
	return &ObservationTracker{byTX: make(map[string]ReceiptObservation)}
}

func observationKey(chainID, transactionHash string) string {
	return chainID + "|" + strings.ToLower(strings.TrimSpace(transactionHash))
}

// Prior returns the stored observation when present in process memory.
func (t *ObservationTracker) Prior(chainID, transactionHash string) (ReceiptObservation, bool) {
	if t == nil {
		return ReceiptObservation{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	value, found := t.byTX[observationKey(chainID, transactionHash)]
	return value, found
}

// Remember stores the latest observation after a verification pass.
func (t *ObservationTracker) Remember(observation ReceiptObservation) {
	if t == nil {
		return
	}
	if observation.ChainID == "" || observation.TransactionHash == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.byTX == nil {
		t.byTX = make(map[string]ReceiptObservation)
	}
	observation.TransactionHash = strings.ToLower(strings.TrimSpace(observation.TransactionHash))
	t.byTX[observationKey(observation.ChainID, observation.TransactionHash)] = observation
}

// Evaluate compares current against the best available prior (process memory
// preferred, else durablePrior from persisted adapter reference), updates
// memory with merged state, and returns the signal plus the merged observation
// that must be persisted for restart safety.
func (t *ObservationTracker) Evaluate(durablePrior, current ReceiptObservation) (ReorgSignal, ReceiptObservation) {
	if current.ChainID == "" {
		current.ChainID = durablePrior.ChainID
	}
	if current.TransactionHash == "" {
		current.TransactionHash = durablePrior.TransactionHash
	}
	processPrior, found := t.Prior(current.ChainID, current.TransactionHash)
	prior := durablePrior
	if found {
		prior = processPrior
	}
	signal := CompareReceiptObservations(prior, current)
	merged := MergeObservation(prior, current)
	if merged.ChainID == "" {
		merged.ChainID = current.ChainID
	}
	if merged.TransactionHash == "" {
		merged.TransactionHash = strings.ToLower(strings.TrimSpace(current.TransactionHash))
	}
	t.Remember(merged)
	return signal, merged
}

// ObservationFromReceipt builds a current-pass observation from a chain receipt.
// Normalized log fingerprint is included when logs are present so successive
// observations can detect log mutation without persisting full log bodies.
func ObservationFromReceipt(receipt Receipt, present bool) ReceiptObservation {
	fp := ""
	if present && len(receipt.Logs) > 0 {
		fp = LogFingerprint(receipt.Logs)
	}
	return ReceiptObservation{
		ChainID:         receipt.ChainID,
		TransactionHash: receipt.TransactionHash,
		Status:          receipt.Status,
		BlockNumber:     receipt.BlockNumber,
		BlockHash:       strings.ToLower(strings.TrimSpace(receipt.BlockHash)),
		Confirmations:   receipt.Confirmations,
		Present:         present,
		Known:           present,
		LogFingerprint:  fp,
	}
}

// ReasonCodeForReorg maps an observation-integrity signal to a safe recovery
// reason code. Codes retain the historical ONCHAIN_REORG_* prefix for stable
// operators; on Arc they mean defensive RPC/observation inconsistency, not
// expected consensus reorg behavior.
func ReasonCodeForReorg(signal ReorgSignal) string {
	switch signal {
	case ReorgReceiptMissing:
		return "ONCHAIN_REORG_RECEIPT_MISSING"
	case ReorgBlockHashChanged:
		return "ONCHAIN_REORG_BLOCK_HASH_CHANGED"
	case ReorgBlockNumberChanged:
		return "ONCHAIN_REORG_BLOCK_NUMBER_CHANGED"
	case ReorgConfirmationsDecreased:
		return "ONCHAIN_REORG_CONFIRMATIONS_DECREASED"
	case ReorgLogsChanged:
		return "ONCHAIN_REORG_LOGS_CHANGED"
	default:
		return "ONCHAIN_REORG_INCONSISTENT"
	}
}

// ValidateObservationKey is a defensive helper for tests and callers.
func ValidateObservationKey(chainID, transactionHash string) error {
	if chainID == "" || transactionHash == "" {
		return fmt.Errorf("observation key requires chain ID and transaction hash")
	}
	return nil
}
