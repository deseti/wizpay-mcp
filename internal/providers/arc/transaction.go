package arc

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Contract executions already supported by the shared Arc verifier can carry
// larger typed calldata. SEND itself still requires exactly 68 bytes in its
// domain verifier; this is only the RPC ingestion bound.
const maxTransactionInputBytes = 64 * 1024

func decodeTransactionInput(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "0x") && !strings.HasPrefix(value, "0X") {
		return nil, fmt.Errorf("transaction input lacks 0x prefix")
	}
	body := value[2:]
	if len(body)%2 != 0 || len(body)/2 > maxTransactionInputBytes {
		return nil, fmt.Errorf("transaction input has invalid size")
	}
	decoded, err := hex.DecodeString(body)
	if err != nil {
		return nil, fmt.Errorf("transaction input is invalid")
	}
	return decoded, nil
}
