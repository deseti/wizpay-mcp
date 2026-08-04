package contracts

import (
	"encoding/hex"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/crypto/sha3"
)

// ValidAddress reports whether value is a well-formed 20-byte EVM address
// (0x-prefixed, 40 hex characters). Zero address is considered well-formed
// syntactically; domain validators reject it where required.
func ValidAddress(value string) bool {
	return hasHexPrefix(value) && len(value) == 42 && common.IsHexAddress(value)
}

func hasHexPrefix(value string) bool {
	return len(value) >= 2 && value[0] == '0' && (value[1] == 'x' || value[1] == 'X')
}

// NormalizeAddress lowercases a valid address. Invalid input is returned as-is
// after TrimSpace so callers can fail closed on comparison mismatches.
func NormalizeAddress(value string) string {
	value = strings.TrimSpace(value)
	if !ValidAddress(value) {
		return value
	}
	return strings.ToLower(value)
}

// ChecksumAddress returns the EIP-55 checksum form of a valid address.
// Invalid input is returned unchanged.
func ChecksumAddress(value string) string {
	value = strings.TrimSpace(value)
	if !ValidAddress(value) {
		return value
	}
	return common.HexToAddress(value).Hex()
}

// AddressesEqual reports case-insensitive equality of two valid addresses.
func AddressesEqual(a, b string) bool {
	if !ValidAddress(a) || !ValidAddress(b) {
		return false
	}
	return NormalizeAddress(a) == NormalizeAddress(b)
}

// MustAddressBytes returns the 20-byte representation of a valid address.
func MustAddressBytes(value string) common.Address {
	return common.HexToAddress(value)
}

// TopicAddress encodes an address as a 32-byte ABI topic (left-padded).
func TopicAddress(value string) []byte {
	addr := common.HexToAddress(value)
	topic := make([]byte, 32)
	copy(topic[12:], addr.Bytes())
	return topic
}

// AddressFromTopic decodes a left-padded address topic.
func AddressFromTopic(topic []byte) (string, bool) {
	if len(topic) != 32 {
		return "", false
	}
	for _, b := range topic[:12] {
		if b != 0 {
			return "", false
		}
	}
	return common.BytesToAddress(topic[12:]).Hex(), true
}

// Keccak256 returns the Keccak-256 hash of data. Used only for ABI selectors
// and event topics — never for signing.
func Keccak256(data []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write(data)
	return h.Sum(nil)
}

// Selector4 returns the 4-byte function selector for a canonical signature.
func Selector4(signature string) [4]byte {
	var out [4]byte
	copy(out[:], Keccak256([]byte(signature))[:4])
	return out
}

// EventTopic0 returns the event signature topic for a canonical event signature.
func EventTopic0(signature string) []byte {
	return Keccak256([]byte(signature))
}

// DecodeHexBytes decodes a 0x-prefixed hex string into bytes.
func DecodeHexBytes(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if !hasHexPrefix(value) {
		return nil, hex.ErrLength
	}
	return hex.DecodeString(value[2:])
}
