package swap_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/contracts/swap"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func TestWizPaySwapExecutedSignatureAndDecode(t *testing.T) {
	event, err := swap.EventBySignature(swap.SigWizPaySwapExecuted)
	if err != nil {
		t.Fatal(err)
	}
	wantTopic := contracts.EventTopic0(swap.SigWizPaySwapExecuted)
	if !bytesEqual(event.ID.Bytes(), wantTopic) {
		t.Fatalf("topic0 mismatch: %x vs %x", event.ID.Bytes(), wantTopic)
	}
	// Indexed: user, router, tokenIn
	if !(event.Inputs[0].Indexed && event.Inputs[1].Indexed && event.Inputs[2].Indexed) {
		t.Fatal("user/router/tokenIn must be indexed per verified ABI")
	}
	for i := 3; i < len(event.Inputs); i++ {
		if event.Inputs[i].Indexed {
			t.Fatalf("%s unexpectedly indexed", event.Inputs[i].Name)
		}
	}

	user := "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	router := "0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	tokenIn := "0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
	data, err := event.Inputs.NonIndexed().Pack(
		common.HexToAddress("0x3600000000000000000000000000000000000001"),
		big.NewInt(1000),
		big.NewInt(10),
		big.NewInt(990),
		big.NewInt(980),
		common.HexToAddress("0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD"),
	)
	if err != nil {
		t.Fatal(err)
	}
	log := contracts.Log{
		Address: contracts.AddressWizPaySwapExecutor,
		Topics: [][]byte{
			event.ID.Bytes(),
			contracts.TopicAddress(user),
			contracts.TopicAddress(router),
			contracts.TopicAddress(tokenIn),
		},
		Data:    data,
		ChainID: contracts.ChainIDArcTestnet,
	}
	decoded, err := swap.DecodeWizPaySwapExecuted(nil, log)
	if err != nil {
		t.Fatal(err)
	}
	if !contracts.AddressesEqual(decoded.User, user) ||
		!contracts.AddressesEqual(decoded.Router, router) ||
		!contracts.AddressesEqual(decoded.TokenIn, tokenIn) {
		t.Fatalf("decoded indexed fields = %#v", decoded)
	}
	if decoded.AmountOut.Cmp(big.NewInt(980)) != 0 || decoded.FeeAmount.Cmp(big.NewInt(10)) != 0 {
		t.Fatalf("decoded non-indexed = %#v", decoded)
	}
}

func TestSwapEventFailures(t *testing.T) {
	event, err := swap.EventBySignature(swap.SigWizPaySwapExecuted)
	if err != nil {
		t.Fatal(err)
	}
	data, err := event.Inputs.NonIndexed().Pack(
		common.HexToAddress("0x3600000000000000000000000000000000000001"),
		big.NewInt(1), big.NewInt(0), big.NewInt(1), big.NewInt(1),
		common.HexToAddress("0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD"),
	)
	if err != nil {
		t.Fatal(err)
	}
	topics := [][]byte{
		event.ID.Bytes(),
		contracts.TopicAddress("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"),
		contracts.TopicAddress("0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"),
		contracts.TopicAddress("0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"),
	}

	t.Run("wrong contract address", func(t *testing.T) {
		log := contracts.Log{
			Address: contracts.AddressWizPayPayroll,
			Topics:  topics,
			Data:    data,
			ChainID: contracts.ChainIDArcTestnet,
		}
		if _, err := swap.DecodeWizPaySwapExecuted(nil, log); !hasCode(err, apperrors.CodeValidationError) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("malformed data", func(t *testing.T) {
		log := contracts.Log{
			Address: contracts.AddressWizPaySwapExecutor,
			Topics:  topics,
			Data:    []byte{0xff},
			ChainID: contracts.ChainIDArcTestnet,
		}
		if _, err := swap.DecodeWizPaySwapExecuted(nil, log); !hasCode(err, apperrors.CodeValidationError) {
			t.Fatalf("error = %v", err)
		}
	})
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
