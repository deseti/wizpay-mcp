package arc

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/providers"
)

const logTestHash = "0x1111111111111111111111111111111111111111111111111111111111111111"
const logTestBlockHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const logTestAddress = "0x87ace45582f45cc81ac1e627e875ae84cbd75946"

func topicHex(b byte) string {
	raw := make([]byte, 32)
	raw[31] = b
	return "0x" + hex.EncodeToString(raw)
}

func validLog() rawReceiptLog {
	return rawReceiptLog{
		Address:         logTestAddress,
		Topics:          []string{topicHex(1), topicHex(2)},
		Data:            "0x" + strings.Repeat("ab", 32),
		LogIndex:        "0x0",
		TransactionHash: logTestHash,
	}
}

func TestNormalizeReceiptLogsSuccess(t *testing.T) {
	logs, err := normalizeReceiptLogs([]rawReceiptLog{validLog()}, logTestHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("len = %d", len(logs))
	}
	if logs[0].Address != logTestAddress {
		t.Fatalf("address = %q", logs[0].Address)
	}
	if len(logs[0].Topics) != 2 || len(logs[0].Topics[0]) != 32 {
		t.Fatalf("topics = %#v", logs[0].Topics)
	}
	if !logs[0].HasIndex || logs[0].Index != 0 {
		t.Fatalf("index = %#v", logs[0])
	}
}

func TestNormalizeReceiptLogsNoLogs(t *testing.T) {
	logs, err := normalizeReceiptLogs(nil, logTestHash)
	if err != nil {
		t.Fatal(err)
	}
	if logs != nil {
		t.Fatalf("expected nil, got %#v", logs)
	}
}

func TestNormalizeReceiptLogsMalformedAddress(t *testing.T) {
	raw := validLog()
	raw.Address = "not-an-address"
	if _, err := normalizeReceiptLogs([]rawReceiptLog{raw}, logTestHash); err == nil {
		t.Fatal("expected malformed address error")
	}
}

func TestNormalizeReceiptLogsMalformedTopic(t *testing.T) {
	raw := validLog()
	raw.Topics = []string{"0x1234"} // too short
	if _, err := normalizeReceiptLogs([]rawReceiptLog{raw}, logTestHash); err == nil {
		t.Fatal("expected malformed topic error")
	}
}

func TestNormalizeReceiptLogsOversizedData(t *testing.T) {
	raw := validLog()
	raw.Data = "0x" + strings.Repeat("aa", providers.MaxLogDataBytes+1)
	if _, err := normalizeReceiptLogs([]rawReceiptLog{raw}, logTestHash); err == nil {
		t.Fatal("expected oversized data error")
	}
}

func TestNormalizeReceiptLogsTooManyLogs(t *testing.T) {
	raw := make([]rawReceiptLog, providers.MaxReceiptLogs+1)
	for i := range raw {
		raw[i] = validLog()
	}
	if _, err := normalizeReceiptLogs(raw, logTestHash); err == nil {
		t.Fatal("expected too many logs error")
	}
}

func TestNormalizeReceiptLogsDeterministicOrdering(t *testing.T) {
	a := validLog()
	a.LogIndex = "0x0"
	a.Topics = []string{topicHex(1)}
	b := validLog()
	b.LogIndex = "0x1"
	b.Topics = []string{topicHex(2)}
	logs, err := normalizeReceiptLogs([]rawReceiptLog{a, b}, logTestHash)
	if err != nil {
		t.Fatal(err)
	}
	if logs[0].Topics[0][31] != 1 || logs[1].Topics[0][31] != 2 {
		t.Fatalf("order not preserved: %#v", logs)
	}
	// Fingerprint is stable for the same order.
	fp1 := providers.LogFingerprint(logs)
	fp2 := providers.LogFingerprint(logs)
	if fp1 == "" || fp1 != fp2 {
		t.Fatalf("fingerprint unstable: %q %q", fp1, fp2)
	}
}

func TestNormalizeReceiptLogsTxHashMismatch(t *testing.T) {
	raw := validLog()
	raw.TransactionHash = "0x2222222222222222222222222222222222222222222222222222222222222222"
	if _, err := normalizeReceiptLogs([]rawReceiptLog{raw}, logTestHash); err == nil {
		t.Fatal("expected tx hash mismatch")
	}
}

func TestTransactionReceiptIncludesLogs(t *testing.T) {
	source := fakeSource{found: true, head: 16, payload: receiptPayload{
		Status: "0x1", BlockNumber: "0x10", BlockHash: logTestBlockHash,
		TransactionHash: logTestHash,
		Logs:            []rawReceiptLog{validLog()},
	}}
	receipt, err := newVerifier(t, source).TransactionReceipt(context.Background(), ChainIDTestnet, logTestHash)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != providers.ReceiptSuccess {
		t.Fatalf("status = %s", receipt.Status)
	}
	if len(receipt.Logs) != 1 {
		t.Fatalf("logs = %d", len(receipt.Logs))
	}
	if receipt.Logs[0].Address != logTestAddress {
		t.Fatalf("log address = %q", receipt.Logs[0].Address)
	}
}

func TestTransactionReceiptRejectsMalformedLogs(t *testing.T) {
	bad := validLog()
	bad.Address = "bad"
	source := fakeSource{found: true, head: 16, payload: receiptPayload{
		Status: "0x1", BlockNumber: "0x10", BlockHash: logTestBlockHash,
		TransactionHash: logTestHash,
		Logs:            []rawReceiptLog{bad},
	}}
	if _, err := newVerifier(t, source).TransactionReceipt(context.Background(), ChainIDTestnet, logTestHash); err == nil {
		t.Fatal("malformed logs must fail closed")
	}
}

func TestTransactionReceiptStatusFailureRetainsNoSuccess(t *testing.T) {
	source := fakeSource{found: true, head: 16, payload: receiptPayload{
		Status: "0x0", BlockNumber: "0x10", BlockHash: logTestBlockHash,
		TransactionHash: logTestHash,
		Logs:            []rawReceiptLog{validLog()},
	}}
	receipt, err := newVerifier(t, source).TransactionReceipt(context.Background(), ChainIDTestnet, logTestHash)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != providers.ReceiptReverted {
		t.Fatalf("status = %s", receipt.Status)
	}
	// Logs still extracted for audit; status remains REVERTED (never SUCCESS).
	if len(receipt.Logs) != 1 {
		t.Fatalf("logs = %d", len(receipt.Logs))
	}
}

func TestTransactionReceiptHashMismatchStillRefused(t *testing.T) {
	other := "0x2222222222222222222222222222222222222222222222222222222222222222"
	source := fakeSource{found: true, head: 100, payload: receiptPayload{
		Status: "0x1", BlockNumber: "0x10", TransactionHash: other,
		Logs: []rawReceiptLog{validLog()},
	}}
	if _, err := newVerifier(t, source).TransactionReceipt(context.Background(), ChainIDTestnet, logTestHash); err == nil {
		t.Fatal("receipt for a different transaction must be refused")
	}
}
