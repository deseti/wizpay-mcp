package providers

import (
	"fmt"
	"strings"
	"sync"
)

// ReceiptObservation is one prior on-chain receipt observation used for
// reorg/inconsistency detection. It carries no provider secrets.
type ReceiptObservation struct {
	ChainID         string
	TransactionHash string
	Status          ReceiptStatus
	BlockNumber     uint64
	BlockHash       string
	Confirmations   uint64
	// Present is true when a receipt body was observed (not merely UNKNOWN-absent).
	Present bool
}

// ReorgSignal classifies a comparison between a prior observation and a new one.
type ReorgSignal string

const (
	// ReorgNone means the new observation is consistent with the prior one
	// (including normal confirmation growth or first observation).
	ReorgNone ReorgSignal = "NONE"
	// ReorgReceiptMissing means a previously present receipt is no longer found.
	ReorgReceiptMissing ReorgSignal = "RECEIPT_MISSING"
	// ReorgBlockHashChanged means the inclusion block hash changed.
	ReorgBlockHashChanged ReorgSignal = "BLOCK_HASH_CHANGED"
	// ReorgBlockNumberChanged means the inclusion block number changed.
	ReorgBlockNumberChanged ReorgSignal = "BLOCK_NUMBER_CHANGED"
	// ReorgConfirmationsDecreased means confirmation depth fell after a prior observation.
	ReorgConfirmationsDecreased ReorgSignal = "CONFIRMATIONS_DECREASED"
)

func (s ReorgSignal) Inconsistent() bool {
	return s != ReorgNone && s != ""
}

// CompareReceiptObservations detects meaningful chain inconsistency between a
// prior observation and a newly read receipt. A first observation always returns
// ReorgNone. Inconsistency never implies resubmission — only reconciliation.
func CompareReceiptObservations(prior, current ReceiptObservation) ReorgSignal {
	if !prior.Present {
		return ReorgNone
	}
	if !current.Present {
		return ReorgReceiptMissing
	}
	if prior.BlockHash != "" && current.BlockHash != "" &&
		!strings.EqualFold(prior.BlockHash, current.BlockHash) {
		return ReorgBlockHashChanged
	}
	if prior.BlockNumber != 0 && current.BlockNumber != 0 && prior.BlockNumber != current.BlockNumber {
		return ReorgBlockNumberChanged
	}
	if prior.Confirmations > 0 && current.Confirmations < prior.Confirmations {
		// Allow equality or growth only. A decrease is a reorg/reorg-like signal.
		return ReorgConfirmationsDecreased
	}
	return ReorgNone
}

// ObservationTracker stores the latest receipt observation per transaction so
// subsequent verification passes can detect reorgs. It is process-local memory
// only; durable financial truth remains PostgreSQL execution state.
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

// Prior returns the stored observation when present.
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

// Evaluate compares current against prior, updates memory, and returns the signal.
func (t *ObservationTracker) Evaluate(current ReceiptObservation) ReorgSignal {
	prior, found := t.Prior(current.ChainID, current.TransactionHash)
	signal := ReorgNone
	if found {
		signal = CompareReceiptObservations(prior, current)
	}
	// Always remember the latest observation so subsequent checks have a baseline.
	// On missing-after-present, remember Present=false so we do not immediately
	// re-fire the same missing signal forever without new evidence, but still
	// retain that we once saw a receipt via leaving block metadata empty.
	t.Remember(current)
	return signal
}

// ObservationFromReceipt builds a tracker observation from a chain receipt.
func ObservationFromReceipt(receipt Receipt, present bool) ReceiptObservation {
	return ReceiptObservation{
		ChainID:         receipt.ChainID,
		TransactionHash: receipt.TransactionHash,
		Status:          receipt.Status,
		BlockNumber:     receipt.BlockNumber,
		BlockHash:       receipt.BlockHash,
		Confirmations:   receipt.Confirmations,
		Present:         present,
	}
}

// ReasonCodeForReorg maps a reorg signal to a safe recovery reason code.
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
