package providers

import (
	"context"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/execution/runtime"
)

func TestCompareReceiptObservations(t *testing.T) {
	prior := ReceiptObservation{
		Present: true, BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 3,
	}
	cases := []struct {
		name    string
		current ReceiptObservation
		want    ReorgSignal
	}{
		{"first observation", ReceiptObservation{Present: true, BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 1}, ReorgNone},
		{"normal growth", ReceiptObservation{Present: true, BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 5}, ReorgNone},
		{"missing", ReceiptObservation{Present: false}, ReorgReceiptMissing},
		{"hash change", ReceiptObservation{Present: true, BlockNumber: 10, BlockHash: "0xbbb", Confirmations: 3}, ReorgBlockHashChanged},
		{"number change", ReceiptObservation{Present: true, BlockNumber: 11, BlockHash: "0xaaa", Confirmations: 3}, ReorgBlockNumberChanged},
		{"depth decrease", ReceiptObservation{Present: true, BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 1}, ReorgConfirmationsDecreased},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			priorForCase := prior
			if tt.name == "first observation" {
				priorForCase = ReceiptObservation{}
			}
			if got := CompareReceiptObservations(priorForCase, tt.current); got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}

type reorgFakeChain struct {
	receipts []Receipt
	errs     []error
	i        int
}

func (f *reorgFakeChain) TransactionReceipt(context.Context, string, string) (Receipt, error) {
	if f.i < len(f.errs) && f.errs[f.i] != nil {
		err := f.errs[f.i]
		f.i++
		return Receipt{}, err
	}
	if f.i >= len(f.receipts) {
		return Receipt{Status: ReceiptUnknown, ChainID: testChainID, TransactionHash: testHash}, nil
	}
	receipt := f.receipts[f.i]
	f.i++
	return receipt, nil
}

type reorgFakeResolver struct{}

func (reorgFakeResolver) ResolveReference(_ context.Context, _ string, reference Reference) (Reference, error) {
	return reference, nil
}

func newReorgTestVerifier(t *testing.T, chain ChainVerifier) *Verifier {
	t.Helper()
	verifier, err := NewVerifier(chain, reorgFakeResolver{}, VerifierConfig{MinConfirmations: 2}, func() time.Time { return providerTestNow })
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}

func referenceWithHash() Reference {
	return Reference{
		Provider: ProviderCircleUserControlled, ChainID: testChainID,
		WalletID: testWalletID, ProviderTransactionID: "tx-1", TransactionHash: testHash,
	}
}

func mustEncodeReference(t *testing.T, reference Reference) string {
	t.Helper()
	encoded, err := reference.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestVerifierReorgReceiptMissingThenNoResubmitSemantics(t *testing.T) {
	chain := &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 3},
		{Status: ReceiptUnknown, ChainID: testChainID, TransactionHash: testHash},
	}}
	verifier := newReorgTestVerifier(t, chain)
	request := testRequest(t, "intent-reorg-missing")
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	encoded := mustEncodeReference(t, referenceWithHash())

	first, err := verifier.Verify(context.Background(), value, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if first.Outcome != runtime.VerificationVerified {
		t.Fatalf("first outcome = %s", first.Outcome)
	}

	second, err := verifier.Verify(context.Background(), value, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if second.Outcome != runtime.VerificationPending {
		t.Fatalf("missing-after-present must be pending, got %s", second.Outcome)
	}
}

func TestVerifierReorgBlockHashAndNumberAndDepth(t *testing.T) {
	base := Receipt{
		Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash,
		BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 4,
	}
	cases := []struct {
		name     string
		next     Receipt
		progress bool
	}{
		{"hash", Receipt{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: "0xbbb", Confirmations: 4}, false},
		{"number", Receipt{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 12, BlockHash: "0xaaa", Confirmations: 4}, false},
		{"depth", Receipt{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 2}, false},
		{"progress", Receipt{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 6}, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			chain := &reorgFakeChain{receipts: []Receipt{base, tt.next}}
			verifier := newReorgTestVerifier(t, chain)
			request := testRequest(t, "intent-reorg-"+tt.name)
			value, err := execution.New(request)
			if err != nil {
				t.Fatal(err)
			}
			encoded := mustEncodeReference(t, referenceWithHash())
			if _, err := verifier.Verify(context.Background(), value, encoded); err != nil {
				t.Fatal(err)
			}
			result, err := verifier.Verify(context.Background(), value, encoded)
			if err != nil {
				t.Fatal(err)
			}
			if tt.progress {
				if result.Outcome != runtime.VerificationVerified {
					t.Fatalf("normal progression should verify, got %s", result.Outcome)
				}
				return
			}
			if result.Outcome != runtime.VerificationPending {
				t.Fatalf("reorg signal should stay pending, got %s", result.Outcome)
			}
		})
	}
}

func TestVerifierRevertedStillFails(t *testing.T) {
	chain := &reorgFakeChain{receipts: []Receipt{{
		Status: ReceiptReverted, ChainID: testChainID, TransactionHash: testHash,
		BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 1,
	}}}
	verifier := newReorgTestVerifier(t, chain)
	request := testRequest(t, "intent-reorg-revert")
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := verifier.Verify(context.Background(), value, mustEncodeReference(t, referenceWithHash()))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != runtime.VerificationFailed {
		t.Fatalf("outcome = %s", result.Outcome)
	}
}

func TestVerifierShallowThenDeepConfirms(t *testing.T) {
	chain := &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptUnknown, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 1},
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 3},
	}}
	verifier := newReorgTestVerifier(t, chain)
	request := testRequest(t, "intent-reorg-progress")
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	encoded := mustEncodeReference(t, referenceWithHash())
	first, err := verifier.Verify(context.Background(), value, encoded)
	if err != nil || first.Outcome != runtime.VerificationPending {
		t.Fatalf("shallow = %#v err=%v", first, err)
	}
	second, err := verifier.Verify(context.Background(), value, encoded)
	if err != nil || second.Outcome != runtime.VerificationVerified {
		t.Fatalf("deep = %#v err=%v", second, err)
	}
}
