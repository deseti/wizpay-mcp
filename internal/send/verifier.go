package send

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	transferSelector = []byte{0xa9, 0x05, 0x9c, 0xbb}
	transferEventID  = crypto.Keccak256([]byte("Transfer(address,address,uint256)"))
)

type Verifier struct{}

func NewVerifier() Verifier { return Verifier{} }

// Verify requires the exact transaction envelope, calldata, and Transfer log.
func (Verifier) Verify(intent intents.Intent, plan Plan, receipt providers.Receipt) error {
	if err := intent.Validate(); err != nil || intent.Type() != intents.TypeSend {
		return fmt.Errorf("invalid SEND intent")
	}
	if plan.intentID != intent.IntentID() || plan.intentDigest != intent.Digest() {
		return fmt.Errorf("send plan does not match frozen intent")
	}
	if receipt.ChainID != contracts.ChainIDArcMainnet || receipt.Status != providers.ReceiptSuccess {
		return fmt.Errorf("send receipt is not a successful Arc Mainnet receipt")
	}
	if !receipt.HasTransaction {
		return fmt.Errorf("send transaction evidence is missing")
	}
	if !contracts.AddressesEqual(receipt.From, plan.wallet) || !contracts.AddressesEqual(receipt.To, plan.token.Address) {
		return fmt.Errorf("send transaction source or token is wrong")
	}
	value, ok := new(big.Int).SetString(strings.TrimPrefix(strings.ToLower(receipt.Value), "0x"), 16)
	if !ok || value.Sign() != 0 {
		return fmt.Errorf("send transaction value must be zero")
	}
	recipient, amount, err := decodeTransfer(receipt.Input)
	if err != nil || !contracts.AddressesEqual(recipient, plan.recipient) || amount.String() != plan.amount.BaseUnits {
		return fmt.Errorf("send calldata does not match frozen intent")
	}
	if !hasTransferEvent(receipt.Logs, plan.token.Address, plan.wallet, plan.recipient, amount) {
		return fmt.Errorf("matching ERC-20 Transfer event is missing")
	}
	return nil
}

func decodeTransfer(input []byte) (string, *big.Int, error) {
	if len(input) != 68 || !bytes.Equal(input[:4], transferSelector) {
		return "", nil, fmt.Errorf("calldata is not exact ERC-20 transfer")
	}
	for _, b := range input[4:16] {
		if b != 0 {
			return "", nil, fmt.Errorf("recipient word is malformed")
		}
	}
	recipient := "0x" + fmt.Sprintf("%x", input[16:36])
	if !contracts.ValidAddress(recipient) || contracts.AddressesEqual(recipient, contracts.AddressZero) {
		return "", nil, fmt.Errorf("recipient is invalid")
	}
	amount := new(big.Int).SetBytes(input[36:68])
	if amount.Sign() <= 0 {
		return "", nil, fmt.Errorf("amount is invalid")
	}
	return recipient, amount, nil
}

func hasTransferEvent(logs []providers.ReceiptLog, token, from, to string, amount *big.Int) bool {
	for _, log := range logs {
		if !contracts.AddressesEqual(log.Address, token) || len(log.Topics) != 3 || !bytes.Equal(log.Topics[0], transferEventID) || len(log.Data) != 32 {
			continue
		}
		if !topicAddressEquals(log.Topics[1], from) || !topicAddressEquals(log.Topics[2], to) || new(big.Int).SetBytes(log.Data).Cmp(amount) != 0 {
			continue
		}
		return true
	}
	return false
}

func topicAddressEquals(topic []byte, address string) bool {
	if len(topic) != 32 {
		return false
	}
	for _, b := range topic[:12] {
		if b != 0 {
			return false
		}
	}
	return strings.EqualFold("0x"+fmt.Sprintf("%x", topic[12:]), address)
}
