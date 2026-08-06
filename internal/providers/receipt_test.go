package providers

import (
	"testing"
)

func TestLogFingerprintEmpty(t *testing.T) {
	if got := LogFingerprint(nil); got != "" {
		t.Fatalf("empty logs fingerprint = %q", got)
	}
}

func TestLogFingerprintChangesWithContent(t *testing.T) {
	topic := make([]byte, 32)
	topic[0] = 1
	a := []ReceiptLog{{Address: "0x1111111111111111111111111111111111111111", Topics: [][]byte{topic}, Data: []byte{1}}}
	b := []ReceiptLog{{Address: "0x1111111111111111111111111111111111111111", Topics: [][]byte{topic}, Data: []byte{2}}}
	if LogFingerprint(a) == LogFingerprint(b) {
		t.Fatal("different data must produce different fingerprints")
	}
	if !LogsEqual(a, a) || LogsEqual(a, b) {
		t.Fatal("LogsEqual mismatch")
	}
}

func TestValidateLogsBounds(t *testing.T) {
	topic := make([]byte, 32)
	good := ReceiptLog{Address: "0x1111111111111111111111111111111111111111", Topics: [][]byte{topic}}
	if err := ValidateLogs([]ReceiptLog{good}); err != nil {
		t.Fatal(err)
	}
	badAddr := good
	badAddr.Address = "nope"
	if err := ValidateLogs([]ReceiptLog{badAddr}); err == nil {
		t.Fatal("bad address must fail")
	}
	tooManyTopics := good
	tooManyTopics.Topics = make([][]byte, MaxLogTopics+1)
	for i := range tooManyTopics.Topics {
		tooManyTopics.Topics[i] = make([]byte, 32)
	}
	if err := ValidateLogs([]ReceiptLog{tooManyTopics}); err == nil {
		t.Fatal("too many topics must fail")
	}
	oversized := good
	oversized.Data = make([]byte, MaxLogDataBytes+1)
	if err := ValidateLogs([]ReceiptLog{oversized}); err == nil {
		t.Fatal("oversized data must fail")
	}
}

func TestCompareReceiptObservationsLogChange(t *testing.T) {
	prior := ReceiptObservation{
		Present: true, Known: true, BlockNumber: 10, BlockHash: "0xaaa",
		Confirmations: 1, LogFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	same := prior
	if got := CompareReceiptObservations(prior, same); got != ReorgNone {
		t.Fatalf("same logs: %s", got)
	}
	changed := prior
	changed.LogFingerprint = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if got := CompareReceiptObservations(prior, changed); got != ReorgLogsChanged {
		t.Fatalf("changed logs: %s", got)
	}
	// Legacy prior without fingerprint does not fail when current adds one.
	legacy := prior
	legacy.LogFingerprint = ""
	if got := CompareReceiptObservations(legacy, changed); got != ReorgNone {
		t.Fatalf("legacy empty fingerprint: %s", got)
	}
	// Logs disappearing while still present is fail-closed.
	missingLogs := prior
	missingLogs.LogFingerprint = ""
	if got := CompareReceiptObservations(prior, missingLogs); got != ReorgLogsChanged {
		t.Fatalf("disappeared logs: %s", got)
	}
}

func TestReferenceLogFingerprintRoundTrip(t *testing.T) {
	fp := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	ref := referenceWithHash().WithReceiptObservation(ReceiptObservation{
		Present: true, Known: true, BlockHash: blockHash(10), BlockNumber: 42,
		Confirmations: 2, LogFingerprint: fp,
	})
	encoded, err := ref.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseReference(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ObsLogFingerprint != fp {
		t.Fatalf("fingerprint = %q", decoded.ObsLogFingerprint)
	}
	obs := decoded.ReceiptObservation()
	if obs.LogFingerprint != fp {
		t.Fatalf("observation fingerprint = %q", obs.LogFingerprint)
	}
}
