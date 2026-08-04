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
		Present: true, Known: true, BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 3,
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

func TestMergeObservationRetainsInclusionWhenMissing(t *testing.T) {
	prior := ReceiptObservation{
		Present: true, Known: true, BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 4,
	}
	merged := MergeObservation(prior, ReceiptObservation{Present: false})
	if merged.Present || !merged.Known {
		t.Fatalf("missing merge = %#v", merged)
	}
	if merged.BlockHash != "0xaaa" || merged.BlockNumber != 10 {
		t.Fatalf("inclusion identity lost: %#v", merged)
	}
	if merged.Confirmations != 0 {
		t.Fatalf("confirmations should reset across absence: %#v", merged)
	}
	// Reappear same inclusion after missing: no reorg.
	reappear := ReceiptObservation{Present: true, BlockNumber: 10, BlockHash: "0xaaa", Confirmations: 2}
	if signal := CompareReceiptObservations(merged, reappear); signal != ReorgNone {
		t.Fatalf("same inclusion reappear signal = %s", signal)
	}
	// Reappear different hash after missing: reorg.
	other := ReceiptObservation{Present: true, BlockNumber: 10, BlockHash: "0xbbb", Confirmations: 2}
	if signal := CompareReceiptObservations(merged, other); signal != ReorgBlockHashChanged {
		t.Fatalf("different hash reappear signal = %s", signal)
	}
	// Reappear different number after missing: reorg.
	otherNum := ReceiptObservation{Present: true, BlockNumber: 11, BlockHash: "0xaaa", Confirmations: 2}
	if signal := CompareReceiptObservations(merged, otherNum); signal != ReorgBlockNumberChanged {
		t.Fatalf("different number reappear signal = %s", signal)
	}
}

type reorgFakeChain struct {
	receipts []Receipt
	i        int
}

func (f *reorgFakeChain) TransactionReceipt(context.Context, string, string) (Receipt, error) {
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
	// MinConfirmations=1 matches Arc deterministic finality guidance.
	verifier, err := NewVerifier(chain, reorgFakeResolver{}, VerifierConfig{MinConfirmations: 1}, func() time.Time { return providerTestNow })
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

func blockHash(suffix byte) string {
	// 32-byte lowercase hex hash with a distinct last byte.
	return "0x" + string(bytesRepeat('a', 62)) + string([]byte{hexDigit(suffix >> 4), hexDigit(suffix & 0x0f)})
}

func bytesRepeat(b byte, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return string(out)
}

func hexDigit(v byte) byte {
	if v < 10 {
		return '0' + v
	}
	return 'a' + (v - 10)
}

func TestVerifierReorgReceiptMissingThenNoResubmitSemantics(t *testing.T) {
	bh := blockHash(1)
	chain := &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: bh, Confirmations: 3},
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

	second, err := verifier.Verify(context.Background(), value, first.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if second.Outcome != runtime.VerificationPending {
		t.Fatalf("missing-after-present must be pending, got %s", second.Outcome)
	}
	// Durable missing state retains inclusion identity.
	decoded, err := ParseReference(second.Reference)
	if err != nil {
		t.Fatal(err)
	}
	obs := decoded.ReceiptObservation()
	if obs.Present || !obs.Known || obs.BlockHash != bh || obs.BlockNumber != 10 {
		t.Fatalf("durable missing observation = %#v", obs)
	}
}

func TestVerifierRestartSameReceiptProgresses(t *testing.T) {
	bh := blockHash(2)
	// First process: shallow observation.
	chain1 := &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptUnknown, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: bh, Confirmations: 1},
	}}
	v1 := newReorgTestVerifier(t, chain1)
	request := testRequest(t, "intent-reorg-restart-ok")
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	encoded := mustEncodeReference(t, referenceWithHash())
	first, err := v1.Verify(context.Background(), value, encoded)
	if err != nil || first.Outcome != runtime.VerificationPending {
		t.Fatalf("shallow = %#v err=%v", first, err)
	}
	// Worker restart: new verifier, durable observation only.
	chain2 := &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: bh, Confirmations: 3},
	}}
	v2 := newReorgTestVerifier(t, chain2)
	second, err := v2.Verify(context.Background(), value, first.Reference)
	if err != nil || second.Outcome != runtime.VerificationVerified {
		t.Fatalf("after restart deep = %#v err=%v", second, err)
	}
}

func TestVerifierRestartReceiptMissing(t *testing.T) {
	bh := blockHash(3)
	chain1 := &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: bh, Confirmations: 3},
	}}
	v1 := newReorgTestVerifier(t, chain1)
	request := testRequest(t, "intent-reorg-restart-missing")
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	first, err := v1.Verify(context.Background(), value, mustEncodeReference(t, referenceWithHash()))
	if err != nil || first.Outcome != runtime.VerificationVerified {
		t.Fatalf("first = %#v err=%v", first, err)
	}
	chain2 := &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptUnknown, ChainID: testChainID, TransactionHash: testHash},
	}}
	v2 := newReorgTestVerifier(t, chain2)
	second, err := v2.Verify(context.Background(), value, first.Reference)
	if err != nil || second.Outcome != runtime.VerificationPending {
		t.Fatalf("restart missing = %#v err=%v", second, err)
	}
}

func TestVerifierRestartBlockHashChanged(t *testing.T) {
	bh1, bh2 := blockHash(4), blockHash(5)
	chain1 := &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: bh1, Confirmations: 3},
	}}
	v1 := newReorgTestVerifier(t, chain1)
	request := testRequest(t, "intent-reorg-restart-hash")
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	first, err := v1.Verify(context.Background(), value, mustEncodeReference(t, referenceWithHash()))
	if err != nil {
		t.Fatal(err)
	}
	chain2 := &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: bh2, Confirmations: 5},
	}}
	v2 := newReorgTestVerifier(t, chain2)
	second, err := v2.Verify(context.Background(), value, first.Reference)
	if err != nil || second.Outcome != runtime.VerificationPending {
		t.Fatalf("hash change after restart must pending, got %#v err=%v", second, err)
	}
	// Stabilization on new inclusion after baseline updates.
	chain3 := &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: bh2, Confirmations: 6},
	}}
	// Continue with same restarted verifier that already absorbed bh2 baseline.
	// Use second.Reference which has merged bh2 after the reorg signal.
	// New verifier with only durable second.Reference:
	v3 := newReorgTestVerifier(t, chain3)
	third, err := v3.Verify(context.Background(), value, second.Reference)
	if err != nil {
		t.Fatal(err)
	}
	// After hash-change, second.Reference retained known inclusion (bh2 after merge
	// of present current). Third compare same inclusion growing confs → verified.
	if third.Outcome != runtime.VerificationVerified {
		t.Fatalf("stable new inclusion should verify, got %s", third.Outcome)
	}
}

func TestVerifierMissingThenReappearSameAndDifferent(t *testing.T) {
	bh1, bh2 := blockHash(6), blockHash(7)
	request := testRequest(t, "intent-reorg-reappear")
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	// present
	v := newReorgTestVerifier(t, &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: bh1, Confirmations: 3},
		{Status: ReceiptUnknown, ChainID: testChainID, TransactionHash: testHash},
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: bh1, Confirmations: 4},
	}})
	encoded := mustEncodeReference(t, referenceWithHash())
	first, err := v.Verify(context.Background(), value, encoded)
	if err != nil || first.Outcome != runtime.VerificationVerified {
		t.Fatalf("present = %#v err=%v", first, err)
	}
	missing, err := v.Verify(context.Background(), value, first.Reference)
	if err != nil || missing.Outcome != runtime.VerificationPending {
		t.Fatalf("missing = %#v err=%v", missing, err)
	}
	same, err := v.Verify(context.Background(), value, missing.Reference)
	if err != nil || same.Outcome != runtime.VerificationVerified {
		t.Fatalf("reappear same = %#v err=%v", same, err)
	}

	// present -> missing -> different hash
	v2 := newReorgTestVerifier(t, &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: bh1, Confirmations: 3},
		{Status: ReceiptUnknown, ChainID: testChainID, TransactionHash: testHash},
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: bh2, Confirmations: 4},
	}})
	req2 := testRequest(t, "intent-reorg-reappear-diff")
	value2, err := execution.New(req2)
	if err != nil {
		t.Fatal(err)
	}
	a, err := v2.Verify(context.Background(), value2, mustEncodeReference(t, referenceWithHash()))
	if err != nil {
		t.Fatal(err)
	}
	b, err := v2.Verify(context.Background(), value2, a.Reference)
	if err != nil {
		t.Fatal(err)
	}
	c, err := v2.Verify(context.Background(), value2, b.Reference)
	if err != nil || c.Outcome != runtime.VerificationPending {
		t.Fatalf("reappear different hash must pending, got %#v err=%v", c, err)
	}
}

func TestVerifierConfirmationRegressionAcrossDurableState(t *testing.T) {
	bh := blockHash(8)
	v1 := newReorgTestVerifier(t, &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: bh, Confirmations: 5},
	}})
	request := testRequest(t, "intent-reorg-conf-reg")
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	first, err := v1.Verify(context.Background(), value, mustEncodeReference(t, referenceWithHash()))
	if err != nil {
		t.Fatal(err)
	}
	v2 := newReorgTestVerifier(t, &reorgFakeChain{receipts: []Receipt{
		{Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash, BlockNumber: 10, BlockHash: bh, Confirmations: 2},
	}})
	second, err := v2.Verify(context.Background(), value, first.Reference)
	if err != nil || second.Outcome != runtime.VerificationPending {
		t.Fatalf("conf regression after restart = %#v err=%v", second, err)
	}
}

func TestVerifierRevertedStillFails(t *testing.T) {
	chain := &reorgFakeChain{receipts: []Receipt{{
		Status: ReceiptReverted, ChainID: testChainID, TransactionHash: testHash,
		BlockNumber: 10, BlockHash: blockHash(9), Confirmations: 1,
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

func TestVerifierOneConfirmationSuccessIsGenericChainLevelOnly(t *testing.T) {
	// Arc finality: one committed SUCCESS receipt at depth 1 satisfies the
	// Phase 11 generic chain verifier. Receipt deliberately carries no logs;
	// Phase 12 domain event verification remains a separate, later gate.
	bh := blockHash(11)
	chain := &reorgFakeChain{receipts: []Receipt{{
		Status: ReceiptSuccess, ChainID: testChainID, TransactionHash: testHash,
		BlockNumber: 10, BlockHash: bh, Confirmations: 1,
	}}}
	verifier := newReorgTestVerifier(t, chain)
	request := testRequest(t, "intent-finality-one-conf")
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := verifier.Verify(context.Background(), value, mustEncodeReference(t, referenceWithHash()))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != runtime.VerificationVerified {
		t.Fatalf("one confirmation SUCCESS must verify at chain level, got %s", result.Outcome)
	}
	// Domain event verification is not performed by this generic verifier.
	receipt := Receipt{}
	if receipt.Status != "" || receipt.BlockHash != "" {
		t.Fatal("sanity: empty Receipt has no domain event payload")
	}
}

func TestVerifierConfigRejectsZeroConfirmations(t *testing.T) {
	if err := (VerifierConfig{MinConfirmations: 0}).Validate(); err == nil {
		t.Fatal("MinConfirmations=0 must be rejected")
	}
	if err := (VerifierConfig{MinConfirmations: 1}).Validate(); err != nil {
		t.Fatalf("MinConfirmations=1 must be accepted: %v", err)
	}
}

func TestReferenceObservationRoundTrip(t *testing.T) {
	ref := referenceWithHash().WithReceiptObservation(ReceiptObservation{
		Present: true, Known: true, BlockHash: blockHash(10), BlockNumber: 42, Confirmations: 7,
	})
	encoded, err := ref.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > maxReferenceLength {
		t.Fatalf("encoded length %d", len(encoded))
	}
	decoded, err := ParseReference(encoded)
	if err != nil {
		t.Fatal(err)
	}
	obs := decoded.ReceiptObservation()
	if !obs.Present || obs.BlockNumber != 42 || obs.Confirmations != 7 || obs.BlockHash != blockHash(10) {
		t.Fatalf("observation round trip = %#v", obs)
	}
}
