package payroll_test

import (
	"bytes"
	"math/big"
	"strings"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/contracts/payroll"
)

func TestMainnetPayrollDescriptorEncodingParity(t *testing.T) {
	same := payroll.SameTokenPayrollInput{Token: contracts.AddressUSDCMainnet, Recipients: []string{"0x1111111111111111111111111111111111111111"}, Amounts: []*big.Int{big.NewInt(1_000_000)}, ReferenceID: "payroll-1"}
	first, err := payroll.EncodeSameTokenPayroll(nil, same)
	if err != nil {
		t.Fatal(err)
	}
	second, err := payroll.EncodeSameTokenPayroll(nil, same)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.CallData(), second.CallData()) {
		t.Fatal("same-token encoding is not deterministic")
	}
	if first.Function() != payroll.SigExecuteSameTokenPayroll || first.Selector() != contracts.Selector4(payroll.SigExecuteSameTokenPayroll) {
		t.Fatalf("unexpected same-token function: %s", first.Function())
	}
	if got := first.Selector(); got != [4]byte{0x6a, 0xaf, 0xf8, 0x34} {
		t.Fatalf("same-token selector = 0x%x", got)
	}
	if first.ChainID() != contracts.ChainIDArcMainnet || first.Network() != contracts.NetworkArcMainnet || !contracts.AddressesEqual(first.To(), contracts.AddressWizPayPayroll) {
		t.Fatalf("unexpected descriptor identity")
	}

	cross := payroll.CrossTokenPayrollInput{TokenIn: contracts.AddressEURCMainnet, TokenOut: contracts.AddressUSDCMainnet, Recipients: same.Recipients, OutputAmounts: same.Amounts, GrossInput: big.NewInt(1_100_000), MinTotalOut: big.NewInt(1_000_000), MinHopPriceX36: big.NewInt(1), Deadline: big.NewInt(1), ReferenceID: "payroll-2"}
	call, err := payroll.EncodeCrossTokenPayroll(nil, cross)
	if err != nil {
		t.Fatal(err)
	}
	if call.Function() != payroll.SigExecuteCrossTokenPayroll || call.Selector() != contracts.Selector4(payroll.SigExecuteCrossTokenPayroll) {
		t.Fatalf("unexpected cross-token function: %s", call.Function())
	}
	if got := call.Selector(); got != [4]byte{0xab, 0xa7, 0x62, 0x52} {
		t.Fatalf("cross-token selector = 0x%x", got)
	}
}

func TestMainnetPayrollDescriptorRejectsUnsupportedResources(t *testing.T) {
	_, err := payroll.EncodeSameTokenPayroll(nil, payroll.SameTokenPayrollInput{Token: "0x2222222222222222222222222222222222222222", Recipients: []string{"0x1111111111111111111111111111111111111111"}, Amounts: []*big.Int{big.NewInt(1)}, ReferenceID: "payroll"})
	if err == nil {
		t.Fatal("unsupported token must fail closed")
	}
	if _, err := payroll.MethodBySignature("batchRouteAndPay(address,address,address[],uint256[],uint256[],string)"); err == nil {
		t.Fatal("Testnet batchRouteAndPay must not remain in the runtime ABI")
	}
}

// TestReferenceIDByteLength is a regression test for the corrective patch that
// replaced utf8.RuneCountInString with len() to match Solidity's
// bytes(referenceId).length check.
//
// A string composed of 4-byte UTF-8 code points (e.g. emoji U+1F4B0) can have
// <=64 runes but >64 bytes, causing on-chain revert while passing the old Go
// validator. The fixed validator must reject any referenceId whose UTF-8 byte
// length exceeds 64.
func TestReferenceIDByteLength(t *testing.T) {
	recipient := []string{"0x1111111111111111111111111111111111111111"}
	amounts := []*big.Int{big.NewInt(1_000_000)}

	// 17 emoji = 17 runes, 68 UTF-8 bytes — must be rejected (>64 bytes).
	// Each U+1F4B0 MONEY BAG encodes to 4 bytes in UTF-8.
	seventeenEmoji := "💰💰💰💰💰💰💰💰💰💰💰💰💰💰💰💰💰" // 17 × U+1F4B0
	if _, err := payroll.EncodeSameTokenPayroll(nil, payroll.SameTokenPayrollInput{
		Token: contracts.AddressUSDCMainnet, Recipients: recipient, Amounts: amounts,
		ReferenceID: seventeenEmoji,
	}); err == nil {
		t.Fatalf("referenceId with 17 emoji (68 bytes, >64) must be rejected to match Solidity bytes(referenceId).length check")
	}

	// 16 emoji = 16 runes, 64 UTF-8 bytes — exactly at the limit, must be accepted.
	exactlyAtLimit := "💰💰💰💰💰💰💰💰💰💰💰💰💰💰💰💰" // 16 × U+1F4B0
	if _, err := payroll.EncodeSameTokenPayroll(nil, payroll.SameTokenPayrollInput{
		Token: contracts.AddressUSDCMainnet, Recipients: recipient, Amounts: amounts,
		ReferenceID: exactlyAtLimit,
	}); err != nil {
		t.Fatalf("referenceId with 16 emoji (64 bytes, ==64) must be accepted: %v", err)
	}

	// 64 ASCII characters — 64 bytes and 64 runes, must be accepted.
	// Built programmatically so byte length is unambiguous across editors.
	ascii64 := strings.Repeat("a", 64)
	if len(ascii64) != 64 {
		t.Fatalf("test setup error: ascii64 has %d bytes, want 64", len(ascii64))
	}
	if _, err := payroll.EncodeSameTokenPayroll(nil, payroll.SameTokenPayrollInput{
		Token: contracts.AddressUSDCMainnet, Recipients: recipient, Amounts: amounts,
		ReferenceID: ascii64,
	}); err != nil {
		t.Fatalf("64-byte ASCII referenceId must be accepted: %v", err)
	}

	// 65 ASCII characters — 65 bytes, must be rejected.
	ascii65 := strings.Repeat("a", 65)
	if _, err := payroll.EncodeSameTokenPayroll(nil, payroll.SameTokenPayrollInput{
		Token: contracts.AddressUSDCMainnet, Recipients: recipient, Amounts: amounts,
		ReferenceID: ascii65,
	}); err == nil {
		t.Fatal("65-byte ASCII referenceId must be rejected")
	}
}
