package payroll_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func TestBatchPaymentRoutedSignatureAndDecode(t *testing.T) {
	event, err := payroll.EventBySignature(payroll.SigBatchPaymentRouted)
	if err != nil {
		t.Fatal(err)
	}
	wantTopic := contracts.EventTopic0(payroll.SigBatchPaymentRouted)
	if !bytesEqual(event.ID.Bytes(), wantTopic) {
		t.Fatalf("topic0 mismatch: %x vs %x", event.ID.Bytes(), wantTopic)
	}
	// Indexed layout from verified ABI: only sender is indexed.
	if len(event.Inputs) != 8 {
		t.Fatalf("input count = %d", len(event.Inputs))
	}
	if !event.Inputs[0].Indexed || event.Inputs[0].Name != "sender" {
		t.Fatalf("sender indexing mismatch: %#v", event.Inputs[0])
	}
	for i := 1; i < len(event.Inputs); i++ {
		if event.Inputs[i].Indexed {
			t.Fatalf("field %s unexpectedly indexed", event.Inputs[i].Name)
		}
	}

	sender := "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	tokenIn := common.HexToAddress("0x3600000000000000000000000000000000000000")
	tokenOut := common.HexToAddress("0x3600000000000000000000000000000000000001")
	data, err := event.Inputs.NonIndexed().Pack(
		tokenIn,
		tokenOut,
		big.NewInt(1000),
		big.NewInt(950),
		big.NewInt(10),
		big.NewInt(2),
		"ref-001",
	)
	if err != nil {
		t.Fatal(err)
	}
	log := contracts.Log{
		Address: contracts.AddressWizPayPayroll,
		Topics:  [][]byte{event.ID.Bytes(), contracts.TopicAddress(sender)},
		Data:    data,
		ChainID: contracts.ChainIDArcTestnet,
	}
	decoded, err := payroll.DecodeBatchPaymentRouted(nil, log)
	if err != nil {
		t.Fatal(err)
	}
	if !contracts.AddressesEqual(decoded.Sender, sender) {
		t.Fatalf("sender = %q", decoded.Sender)
	}
	if decoded.ReferenceID != "ref-001" || decoded.TotalAmountIn.Cmp(big.NewInt(1000)) != 0 {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestPaymentRoutedSignatureAndDecode(t *testing.T) {
	event, err := payroll.EventBySignature(payroll.SigPaymentRouted)
	if err != nil {
		t.Fatal(err)
	}
	wantTopic := contracts.EventTopic0(payroll.SigPaymentRouted)
	if !bytesEqual(event.ID.Bytes(), wantTopic) {
		t.Fatalf("topic0 mismatch")
	}
	if !event.Inputs[0].Indexed || !event.Inputs[1].Indexed {
		t.Fatal("sender and recipient must be indexed")
	}
	for i := 2; i < len(event.Inputs); i++ {
		if event.Inputs[i].Indexed {
			t.Fatalf("%s unexpectedly indexed", event.Inputs[i].Name)
		}
	}

	sender := "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	recipient := "0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	data, err := event.Inputs.NonIndexed().Pack(
		common.HexToAddress("0x3600000000000000000000000000000000000000"),
		common.HexToAddress("0x3600000000000000000000000000000000000001"),
		big.NewInt(500),
		big.NewInt(480),
		big.NewInt(5),
	)
	if err != nil {
		t.Fatal(err)
	}
	log := contracts.Log{
		Address: contracts.AddressWizPayPayroll,
		Topics: [][]byte{
			event.ID.Bytes(),
			contracts.TopicAddress(sender),
			contracts.TopicAddress(recipient),
		},
		Data:    data,
		ChainID: contracts.ChainIDArcTestnet,
	}
	decoded, err := payroll.DecodePaymentRouted(nil, log)
	if err != nil {
		t.Fatal(err)
	}
	if !contracts.AddressesEqual(decoded.Recipient, recipient) || decoded.AmountOut.Cmp(big.NewInt(480)) != 0 {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestPayrollEventFailures(t *testing.T) {
	event, err := payroll.EventBySignature(payroll.SigBatchPaymentRouted)
	if err != nil {
		t.Fatal(err)
	}
	goodData, err := event.Inputs.NonIndexed().Pack(
		common.HexToAddress("0x3600000000000000000000000000000000000000"),
		common.HexToAddress("0x3600000000000000000000000000000000000001"),
		big.NewInt(1), big.NewInt(1), big.NewInt(0), big.NewInt(1), "r",
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("wrong contract address", func(t *testing.T) {
		log := contracts.Log{
			Address: contracts.AddressWizPaySwapExecutor,
			Topics:  [][]byte{event.ID.Bytes(), contracts.TopicAddress("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")},
			Data:    goodData,
			ChainID: contracts.ChainIDArcTestnet,
		}
		if _, err := payroll.DecodeBatchPaymentRouted(nil, log); !hasCode(err, apperrors.CodeValidationError) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("malformed data", func(t *testing.T) {
		log := contracts.Log{
			Address: contracts.AddressWizPayPayroll,
			Topics:  [][]byte{event.ID.Bytes(), contracts.TopicAddress("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")},
			Data:    []byte{0x01, 0x02},
			ChainID: contracts.ChainIDArcTestnet,
		}
		if _, err := payroll.DecodeBatchPaymentRouted(nil, log); !hasCode(err, apperrors.CodeValidationError) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("wrong topic0", func(t *testing.T) {
		badTopic := make([]byte, 32)
		log := contracts.Log{
			Address: contracts.AddressWizPayPayroll,
			Topics:  [][]byte{badTopic, contracts.TopicAddress("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")},
			Data:    goodData,
			ChainID: contracts.ChainIDArcTestnet,
		}
		if _, err := payroll.DecodeBatchPaymentRouted(nil, log); !hasCode(err, apperrors.CodeValidationError) {
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
