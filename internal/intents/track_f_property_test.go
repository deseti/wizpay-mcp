package intents

import (
	"strings"
	"testing"
)

func FuzzTrackFPayrollReferenceUTF8ByteBound(f *testing.F) {
	for _, seed := range []uint8{1, 21, 22, 64, 65} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, count uint8) {
		// U+754C is three UTF-8 bytes. This proves the contract bound is bytes,
		// not runes, without introducing invalid UTF-8 into the financial path.
		reference := strings.Repeat("界", int(count))
		err := validatePayrollReferenceID(reference)
		wantValid := count > 0 && len(reference) <= maxPayrollReferenceIDLength
		if wantValid && err != nil {
			t.Fatalf("%d-byte reference rejected: %v", len(reference), err)
		}
		if !wantValid && err == nil {
			t.Fatalf("%d-byte reference accepted", len(reference))
		}
	})
}
