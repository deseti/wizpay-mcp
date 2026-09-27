package arc

import "testing"

func TestDecodeTransactionInputIsBoundedButSupportsTypedContracts(t *testing.T) {
	if value, err := decodeTransactionInput("0xa9059cbb" + string(make([]byte, 0))); err != nil || len(value) != 4 {
		t.Fatalf("selector decode = %x, %v", value, err)
	}
	large := "0x" + repeatHex("00", 1024)
	if value, err := decodeTransactionInput(large); err != nil || len(value) != 1024 {
		t.Fatalf("typed contract input decode = %d, %v", len(value), err)
	}
	tooLarge := "0x" + repeatHex("00", maxTransactionInputBytes+1)
	if _, err := decodeTransactionInput(tooLarge); err == nil {
		t.Fatal("oversized transaction input accepted")
	}
}

func repeatHex(value string, count int) string {
	result := make([]byte, len(value)*count)
	for i := 0; i < count; i++ {
		copy(result[i*len(value):], value)
	}
	return string(result)
}
