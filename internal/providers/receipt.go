package providers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

// Bounds for receipt log evidence extracted from a known transaction receipt.
// These prevent oversized or malformed provider payloads from entering domain
// verification or durable observation fingerprints.
const (
	// MaxReceiptLogs is the maximum number of logs accepted from one receipt.
	MaxReceiptLogs = 64
	// MaxLogTopics is the EVM maximum topics per log (topic0 + up to 3 indexed).
	MaxLogTopics = 4
	// MaxLogDataBytes bounds non-indexed log data per log.
	MaxLogDataBytes = 8192
)

// ReceiptLog is one bounded, normalized log from a known transaction receipt.
//
// It is observation evidence only: presence of a log never asserts financial
// success. Domain packages decode trusted event shapes from these fields.
// There is no arbitrary eth_getLogs search surface and no generic ABI decoder.
type ReceiptLog struct {
	// Address is the emitting contract (0x-prefixed, 20 bytes).
	Address string
	// Topics are 32-byte topic values in receipt order (topic0 first).
	Topics [][]byte
	// Data is non-indexed log data (may be empty).
	Data []byte
	// Index is the log index within the receipt when the provider reports one.
	// HasIndex distinguishes "0" from "absent".
	Index    uint64
	HasIndex bool
	// TransactionHash is the enclosing transaction hash when present on the log.
	TransactionHash string
}

// Validate checks structural bounds for one receipt log. It does not decode
// event semantics.
func (l ReceiptLog) Validate() error {
	if !ValidAddress(l.Address) {
		return fmt.Errorf("receipt log address is invalid")
	}
	if len(l.Topics) > MaxLogTopics {
		return fmt.Errorf("receipt log exceeds max topics")
	}
	for i, topic := range l.Topics {
		if len(topic) != 32 {
			return fmt.Errorf("receipt log topic %d is malformed", i)
		}
	}
	if len(l.Data) > MaxLogDataBytes {
		return fmt.Errorf("receipt log data exceeds max size")
	}
	if l.TransactionHash != "" && !ValidTransactionHash(l.TransactionHash) {
		return fmt.Errorf("receipt log transaction hash is invalid")
	}
	return nil
}

// Clone returns a deep copy of the log.
func (l ReceiptLog) Clone() ReceiptLog {
	out := ReceiptLog{
		Address:         strings.ToLower(strings.TrimSpace(l.Address)),
		Index:           l.Index,
		HasIndex:        l.HasIndex,
		TransactionHash: strings.ToLower(strings.TrimSpace(l.TransactionHash)),
	}
	if len(l.Topics) > 0 {
		out.Topics = make([][]byte, len(l.Topics))
		for i, topic := range l.Topics {
			if topic == nil {
				continue
			}
			out.Topics[i] = append([]byte(nil), topic...)
		}
	}
	if len(l.Data) > 0 {
		out.Data = append([]byte(nil), l.Data...)
	}
	return out
}

// ContractLog converts provider-neutral log evidence into the contracts decoder
// shape. ChainID is attached by the caller when known.
func (l ReceiptLog) ContractLog(chainID string) contracts.Log {
	topics := make([][]byte, len(l.Topics))
	for i, topic := range l.Topics {
		topics[i] = append([]byte(nil), topic...)
	}
	return contracts.Log{
		Address: l.Address,
		Topics:  topics,
		Data:    append([]byte(nil), l.Data...),
		ChainID: chainID,
	}
}

// ValidateLogs applies receipt-level log bounds. Nil or empty is valid.
// Logs must already be in deterministic receipt order (provider order).
func ValidateLogs(logs []ReceiptLog) error {
	if len(logs) > MaxReceiptLogs {
		return fmt.Errorf("receipt exceeds max logs (%d)", MaxReceiptLogs)
	}
	for i, log := range logs {
		if err := log.Validate(); err != nil {
			return fmt.Errorf("receipt log %d: %w", i, err)
		}
	}
	return nil
}

// CloneLogs returns a deep copy of logs in the same order.
func CloneLogs(logs []ReceiptLog) []ReceiptLog {
	if len(logs) == 0 {
		return nil
	}
	out := make([]ReceiptLog, len(logs))
	for i, log := range logs {
		out[i] = log.Clone()
	}
	return out
}

// LogFingerprint returns a deterministic hex SHA-256 over normalized logs.
// Empty log sets return an empty fingerprint (not a hash of empty input) so
// "no logs observed" stays distinguishable from a hashed empty payload only
// when callers check HasLogs separately; both empty fingerprints still match.
//
// The fingerprint is used for observation-integrity comparison across
// verification passes. It is not financial success evidence.
func LogFingerprint(logs []ReceiptLog) string {
	if len(logs) == 0 {
		return ""
	}
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "wzp-receipt-logs-v1\n%d\n", len(logs))
	for _, log := range logs {
		addr := strings.ToLower(strings.TrimSpace(log.Address))
		_, _ = fmt.Fprintf(h, "a=%s\n", addr)
		_, _ = fmt.Fprintf(h, "t=%d\n", len(log.Topics))
		for _, topic := range log.Topics {
			_, _ = h.Write([]byte("topic:"))
			_, _ = h.Write(topic)
			_, _ = h.Write([]byte{'\n'})
		}
		_, _ = fmt.Fprintf(h, "d=%d\n", len(log.Data))
		_, _ = h.Write(log.Data)
		_, _ = h.Write([]byte{'\n'})
		if log.HasIndex {
			_, _ = fmt.Fprintf(h, "i=%d\n", log.Index)
		}
		if log.TransactionHash != "" {
			_, _ = fmt.Fprintf(h, "h=%s\n", strings.ToLower(log.TransactionHash))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// LogsEqual reports whether two log slices are byte-identical after
// normalization (address lowercasing, topic/data bytes).
func LogsEqual(a, b []ReceiptLog) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i].Address, b[i].Address) {
			return false
		}
		if len(a[i].Topics) != len(b[i].Topics) {
			return false
		}
		for j := range a[i].Topics {
			if !bytes.Equal(a[i].Topics[j], b[i].Topics[j]) {
				return false
			}
		}
		if !bytes.Equal(a[i].Data, b[i].Data) {
			return false
		}
	}
	return true
}

// ContractLogs converts receipt logs for trusted static event decoders.
func ContractLogs(logs []ReceiptLog, chainID string) []contracts.Log {
	if len(logs) == 0 {
		return nil
	}
	out := make([]contracts.Log, len(logs))
	for i, log := range logs {
		out[i] = log.ContractLog(chainID)
	}
	return out
}
