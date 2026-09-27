package swap_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/contracts/swap"
)

func TestWizPayMainnetSwapExecutedSignatureAndDecode(t *testing.T) {
	event, err := swap.EventBySignature(swap.SigWizPayMainnetSwapExecuted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(event.ID.Bytes(), contracts.EventTopic0(swap.SigWizPayMainnetSwapExecuted)) {
		t.Fatal("topic mismatch")
	}
	data, err := event.Inputs.NonIndexed().Pack(big.NewInt(1000), big.NewInt(10), big.NewInt(990), big.NewInt(980), big.NewInt(970))
	if err != nil {
		t.Fatal(err)
	}
	log := contracts.Log{Address: contracts.AddressWizPaySwapExecutor, ChainID: contracts.ChainIDArcMainnet, Topics: [][]byte{event.ID.Bytes(), contracts.TopicAddress("0x1111111111111111111111111111111111111111"), contracts.TopicAddress(contracts.AddressEURCMainnet), contracts.TopicAddress(contracts.AddressUSDCMainnet)}, Data: data}
	decoded, err := swap.DecodeWizPayMainnetSwapExecuted(nil, log)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.AmountOut.Cmp(big.NewInt(980)) != 0 || !contracts.AddressesEqual(decoded.TokenOut, common.HexToAddress(contracts.AddressUSDCMainnet).Hex()) {
		t.Fatalf("decoded = %#v", decoded)
	}
}
