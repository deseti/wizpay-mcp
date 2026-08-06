package arc

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/deseti/wizpay-mcp/internal/providers"
)

// rawReceiptLog is the JSON-RPC eth_getTransactionReceipt log object shape.
// Only fields needed for domain verification are unmarshaled.
type rawReceiptLog struct {
	Address         string   `json:"address"`
	Topics          []string `json:"topics"`
	Data            string   `json:"data"`
	LogIndex        string   `json:"logIndex"`
	TransactionHash string   `json:"transactionHash"`
}

// receiptPayload is the subset of an EVM transaction receipt needed for
// chain-level and domain verification. Logs are bounded and normalized; the
// full raw JSON-RPC body is never retained.
type receiptPayload struct {
	Status          string          `json:"status"`
	TransactionHash string          `json:"transactionHash"`
	BlockNumber     string          `json:"blockNumber"`
	BlockHash       string          `json:"blockHash"`
	Logs            []rawReceiptLog `json:"logs"`
}

// normalizeReceiptLogs converts raw RPC logs into provider-neutral evidence.
//
// Fail-closed rules:
//   - too many logs
//   - malformed address
//   - malformed topic (must be 32-byte 0x-hex)
//   - oversized data
//   - too many topics per log
//
// Ordering is preserved as returned by the provider (receipt order).
// An empty/missing logs array yields a nil slice (valid).
func normalizeReceiptLogs(raw []rawReceiptLog, expectedTxHash string) ([]providers.ReceiptLog, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > providers.MaxReceiptLogs {
		return nil, fmt.Errorf("Arc receipt has too many logs")
	}
	expectedTxHash = strings.ToLower(strings.TrimSpace(expectedTxHash))
	out := make([]providers.ReceiptLog, 0, len(raw))
	for i, item := range raw {
		log, err := normalizeOneReceiptLog(item, expectedTxHash)
		if err != nil {
			return nil, fmt.Errorf("Arc receipt log %d: %w", i, err)
		}
		out = append(out, log)
	}
	if err := providers.ValidateLogs(out); err != nil {
		return nil, fmt.Errorf("Arc receipt logs invalid: %w", err)
	}
	return out, nil
}

func normalizeOneReceiptLog(raw rawReceiptLog, expectedTxHash string) (providers.ReceiptLog, error) {
	address := strings.TrimSpace(raw.Address)
	if !providers.ValidAddress(address) {
		return providers.ReceiptLog{}, fmt.Errorf("malformed address")
	}
	if len(raw.Topics) > providers.MaxLogTopics {
		return providers.ReceiptLog{}, fmt.Errorf("too many topics")
	}
	topics := make([][]byte, 0, len(raw.Topics))
	for j, topicHex := range raw.Topics {
		topic, err := decodeTopic32(topicHex)
		if err != nil {
			return providers.ReceiptLog{}, fmt.Errorf("malformed topic %d", j)
		}
		topics = append(topics, topic)
	}
	data, err := decodeLogData(raw.Data)
	if err != nil {
		return providers.ReceiptLog{}, err
	}
	if len(data) > providers.MaxLogDataBytes {
		return providers.ReceiptLog{}, fmt.Errorf("oversized data")
	}

	log := providers.ReceiptLog{
		Address: strings.ToLower(address),
		Topics:  topics,
		Data:    data,
	}
	if trimmed := strings.TrimSpace(raw.LogIndex); trimmed != "" {
		index, err := parseQuantity(trimmed)
		if err != nil {
			return providers.ReceiptLog{}, fmt.Errorf("malformed log index")
		}
		log.Index = index
		log.HasIndex = true
	}
	if trimmed := strings.ToLower(strings.TrimSpace(raw.TransactionHash)); trimmed != "" {
		if !providers.ValidTransactionHash(trimmed) {
			return providers.ReceiptLog{}, fmt.Errorf("malformed transaction hash")
		}
		if expectedTxHash != "" && trimmed != expectedTxHash {
			return providers.ReceiptLog{}, fmt.Errorf("transaction hash mismatch")
		}
		log.TransactionHash = trimmed
	}
	return log, nil
}

func decodeTopic32(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "0x") && !strings.HasPrefix(value, "0X") {
		return nil, fmt.Errorf("topic missing 0x prefix")
	}
	// Topics must be exactly 32 bytes (64 hex chars after 0x).
	hexBody := value[2:]
	if len(hexBody) != 64 {
		return nil, fmt.Errorf("topic length")
	}
	// Accept mixed case from RPC; store raw bytes.
	decoded, err := hex.DecodeString(hexBody)
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("topic hex")
	}
	return decoded, nil
}

func decodeLogData(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "0x" || value == "0X" {
		return nil, nil
	}
	if !strings.HasPrefix(value, "0x") && !strings.HasPrefix(value, "0X") {
		return nil, fmt.Errorf("malformed data")
	}
	hexBody := value[2:]
	if len(hexBody)%2 != 0 {
		return nil, fmt.Errorf("malformed data")
	}
	// Bound check before decode to avoid large allocations from hostile payloads.
	if len(hexBody)/2 > providers.MaxLogDataBytes {
		return nil, fmt.Errorf("oversized data")
	}
	decoded, err := hex.DecodeString(hexBody)
	if err != nil {
		return nil, fmt.Errorf("malformed data")
	}
	return decoded, nil
}
