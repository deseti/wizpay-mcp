package arc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/providers"
)

const verifierHash = "0x1111111111111111111111111111111111111111111111111111111111111111"

// fakeSource is a deterministic in-memory receipt source.
type fakeSource struct {
	payload    receiptPayload
	found      bool
	receiptErr error
	head       uint64
	headErr    error
}

func (f fakeSource) Receipt(context.Context, string) (receiptPayload, bool, error) {
	return f.payload, f.found, f.receiptErr
}

func (f fakeSource) BlockNumber(context.Context) (uint64, error) {
	return f.head, f.headErr
}

func verifierConfig() Config {
	return Config{
		Enabled: true, ChainID: ChainIDTestnet, Network: NetworkTestnet,
		RPCURL: RPCTestnet, ExplorerURL: ExplorerTestnet, MinConfirmations: 1, Timeout: 15 * time.Second,
	}
}

func newVerifier(t *testing.T, source receiptSource) *Verifier {
	t.Helper()
	verifier, err := NewVerifier(verifierConfig(), source)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return verifier
}

func TestNewVerifierRejectsDisabledAndNilSource(t *testing.T) {
	if _, err := NewVerifier(Config{Enabled: false}, fakeSource{}); err == nil {
		t.Fatalf("a disabled config must be rejected")
	}
	if _, err := NewVerifier(verifierConfig(), nil); err == nil {
		t.Fatalf("a nil source must be rejected")
	}
}

func TestTransactionReceiptRejectsForeignChainAndBadHash(t *testing.T) {
	verifier := newVerifier(t, fakeSource{})
	if _, err := verifier.TransactionReceipt(context.Background(), "1", verifierHash); err == nil {
		t.Fatalf("evidence from another chain must be refused")
	}
	if _, err := verifier.TransactionReceipt(context.Background(), ChainIDTestnet, "not-a-hash"); err == nil {
		t.Fatalf("an invalid transaction hash must be refused")
	}
}

func TestTransactionReceiptPendingCases(t *testing.T) {
	cases := map[string]fakeSource{
		"absent receipt":        {found: false},
		"receipt without block": {found: true, payload: receiptPayload{Status: "0x1", TransactionHash: verifierHash}},
		"head lags receipt":     {found: true, head: 5, payload: receiptPayload{Status: "0x1", BlockNumber: "0x10", TransactionHash: verifierHash}},
		"unrecognized status":   {found: true, head: 100, payload: receiptPayload{Status: "0x2", BlockNumber: "0x10", TransactionHash: verifierHash}},
	}
	for name, source := range cases {
		receipt, err := newVerifier(t, source).TransactionReceipt(context.Background(), ChainIDTestnet, verifierHash)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		if receipt.Status != providers.ReceiptUnknown {
			t.Fatalf("%s: expected UNKNOWN, got %s", name, receipt.Status)
		}
	}
}

func TestTransactionReceiptSuccessAtOneConfirmation(t *testing.T) {
	// Arc deterministic finality: one committed SUCCESS receipt (head == block
	// → confirmations = 1) satisfies the generic chain-level verifier.
	// block 0x10 == 16, head 16 → confirmations = 16-16+1 = 1.
	source := fakeSource{found: true, head: 16, payload: receiptPayload{
		Status: "0x1", BlockNumber: "0x10", BlockHash: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		TransactionHash: verifierHash,
	}}
	receipt, err := newVerifier(t, source).TransactionReceipt(context.Background(), ChainIDTestnet, verifierHash)
	if err != nil {
		t.Fatalf("TransactionReceipt: %v", err)
	}
	if receipt.Status != providers.ReceiptSuccess {
		t.Fatalf("expected SUCCESS, got %s", receipt.Status)
	}
	if receipt.Confirmations != 1 {
		t.Fatalf("expected 1 confirmation, got %d", receipt.Confirmations)
	}
}

func TestTransactionReceiptSuccessDeeperConfirmations(t *testing.T) {
	// block 0x10 == 16, head 17 → confirmations = 2 (still success with min=1).
	source := fakeSource{found: true, head: 17, payload: receiptPayload{Status: "0x1", BlockNumber: "0x10", TransactionHash: verifierHash}}
	receipt, err := newVerifier(t, source).TransactionReceipt(context.Background(), ChainIDTestnet, verifierHash)
	if err != nil {
		t.Fatalf("TransactionReceipt: %v", err)
	}
	if receipt.Status != providers.ReceiptSuccess {
		t.Fatalf("expected SUCCESS, got %s", receipt.Status)
	}
	if receipt.Confirmations != 2 {
		t.Fatalf("expected 2 confirmations, got %d", receipt.Confirmations)
	}
}

func TestTransactionReceiptRequiresConfiguredDepth(t *testing.T) {
	// When operators raise MinConfirmations above 1, shallower receipts stay pending.
	config := verifierConfig()
	config.MinConfirmations = 2
	source := fakeSource{found: true, head: 16, payload: receiptPayload{Status: "0x1", BlockNumber: "0x10", TransactionHash: verifierHash}}
	verifier, err := NewVerifier(config, source)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	receipt, err := verifier.TransactionReceipt(context.Background(), ChainIDTestnet, verifierHash)
	if err != nil {
		t.Fatalf("TransactionReceipt: %v", err)
	}
	if receipt.Status != providers.ReceiptUnknown {
		t.Fatalf("expected UNKNOWN for conf 1 < min 2, got %s", receipt.Status)
	}
	if receipt.Confirmations != 1 || receipt.BlockNumber != 16 {
		t.Fatalf("shallow receipt must retain inclusion baseline: %#v", receipt)
	}
}

func TestTransactionReceiptReverted(t *testing.T) {
	// A revert is final regardless of confirmation depth.
	source := fakeSource{found: true, head: 16, payload: receiptPayload{Status: "0x0", BlockNumber: "0x10", TransactionHash: verifierHash}}
	receipt, err := newVerifier(t, source).TransactionReceipt(context.Background(), ChainIDTestnet, verifierHash)
	if err != nil {
		t.Fatalf("TransactionReceipt: %v", err)
	}
	if receipt.Status != providers.ReceiptReverted {
		t.Fatalf("expected REVERTED, got %s", receipt.Status)
	}
}

func TestTransactionReceiptHashMismatch(t *testing.T) {
	other := "0x2222222222222222222222222222222222222222222222222222222222222222"
	source := fakeSource{found: true, head: 100, payload: receiptPayload{Status: "0x1", BlockNumber: "0x10", TransactionHash: other}}
	if _, err := newVerifier(t, source).TransactionReceipt(context.Background(), ChainIDTestnet, verifierHash); err == nil {
		t.Fatalf("a receipt for a different transaction must be refused")
	}
}

func TestTransactionReceiptPropagatesSourceErrors(t *testing.T) {
	source := fakeSource{receiptErr: fmt.Errorf("boom")}
	if _, err := newVerifier(t, source).TransactionReceipt(context.Background(), ChainIDTestnet, verifierHash); err == nil {
		t.Fatalf("a source error must propagate")
	}
}

func TestExplorerURL(t *testing.T) {
	verifier := newVerifier(t, fakeSource{})
	if got := verifier.ExplorerURL(verifierHash); got != ExplorerTestnet+"/tx/"+verifierHash {
		t.Fatalf("unexpected explorer URL %q", got)
	}
	if verifier.ExplorerURL("not-a-hash") != "" {
		t.Fatalf("an invalid hash must produce no explorer URL")
	}
}
