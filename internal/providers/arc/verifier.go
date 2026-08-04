package arc

import (
	"context"
	"fmt"
	"strings"

	"github.com/deseti/wizpay-mcp/internal/providers"
)

// receiptSource is the read-only chain surface the verifier depends on.
type receiptSource interface {
	Receipt(ctx context.Context, transactionHash string) (receiptPayload, bool, error)
	BlockNumber(ctx context.Context) (uint64, error)
}

// Verifier reads Arc transaction receipts and reports provider-neutral on-chain
// outcomes.
//
// It is the only component permitted to assert that an execution succeeded or
// failed on-chain. It does so from a receipt on the configured chain at the
// configured confirmation depth, and from nothing else.
type Verifier struct {
	source receiptSource
	config Config
}

func NewVerifier(config Config, source receiptSource) (*Verifier, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if !config.Enabled {
		return nil, fmt.Errorf("Arc chain adapter is not enabled")
	}
	if source == nil {
		return nil, fmt.Errorf("Arc receipt source is required")
	}
	return &Verifier{source: source, config: config}, nil
}

var _ providers.ChainVerifier = (*Verifier)(nil)

// TransactionReceipt returns the on-chain outcome of one transaction.
//
// An absent receipt is reported as unknown, never as failure: an unmined
// transaction may still confirm, and treating it as failed would abandon a
// possibly-live transfer. Only an explicit failure status reverts an execution.
func (v *Verifier) TransactionReceipt(ctx context.Context, chainID, transactionHash string) (providers.Receipt, error) {
	if chainID != v.config.ChainID {
		// Evidence from another chain is never acceptable for this chain.
		return providers.Receipt{}, fmt.Errorf("Arc verifier does not serve chain %q", chainID)
	}
	normalized := strings.ToLower(strings.TrimSpace(transactionHash))
	if !providers.ValidTransactionHash(normalized) {
		return providers.Receipt{}, fmt.Errorf("Arc transaction hash is invalid")
	}
	pending := providers.Receipt{Status: providers.ReceiptUnknown, ChainID: chainID, TransactionHash: normalized}

	payload, found, err := v.source.Receipt(ctx, normalized)
	if err != nil {
		return providers.Receipt{}, err
	}
	if !found {
		return pending, nil
	}
	if observed := strings.ToLower(payload.TransactionHash); observed != "" && observed != normalized {
		return providers.Receipt{}, fmt.Errorf("Arc receipt does not match the requested transaction")
	}
	blockNumber, err := parseQuantity(payload.BlockNumber)
	if err != nil {
		// A receipt without a usable block number is not yet usable evidence.
		return pending, nil
	}
	head, err := v.source.BlockNumber(ctx)
	if err != nil {
		return providers.Receipt{}, err
	}
	if head < blockNumber {
		// The head lags the receipt, so depth cannot be established yet.
		return pending, nil
	}
	blockHash := strings.ToLower(strings.TrimSpace(payload.BlockHash))
	receipt := providers.Receipt{
		ChainID:         chainID,
		TransactionHash: normalized,
		BlockNumber:     blockNumber,
		BlockHash:       blockHash,
		Confirmations:   head - blockNumber + 1,
	}
	switch strings.ToLower(strings.TrimSpace(payload.Status)) {
	case "0x1":
		receipt.Status = providers.ReceiptSuccess
	case "0x0":
		receipt.Status = providers.ReceiptReverted
	default:
		// An unrecognized status is never interpreted as either outcome.
		return providers.Receipt{
			Status: providers.ReceiptUnknown, ChainID: chainID, TransactionHash: normalized,
			BlockNumber: blockNumber, BlockHash: blockHash, Confirmations: receipt.Confirmations,
		}, nil
	}
	if receipt.Status == providers.ReceiptSuccess && receipt.Confirmations < v.config.MinConfirmations {
		// Confirmed too shallow to be treated as final, but block identity is
		// retained so reorg detection still has a baseline.
		return providers.Receipt{
			Status: providers.ReceiptUnknown, ChainID: chainID, TransactionHash: normalized,
			BlockNumber: blockNumber, BlockHash: blockHash, Confirmations: receipt.Confirmations,
		}, nil
	}
	return receipt, nil
}

// ExplorerURL returns a human-facing link for a verified transaction. It is
// informational only and is never used as verification evidence.
func (v *Verifier) ExplorerURL(transactionHash string) string {
	normalized := strings.ToLower(strings.TrimSpace(transactionHash))
	if !providers.ValidTransactionHash(normalized) {
		return ""
	}
	return strings.TrimSuffix(v.config.ExplorerURL, "/") + "/tx/" + normalized
}
