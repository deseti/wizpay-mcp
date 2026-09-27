package payroll_test

import (
	"math/big"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/contracts/payroll"
)

func TestDecodeCrossTokenPayrollCanonicalRoundTrip(t *testing.T) {
	input := payroll.CrossTokenPayrollInput{TokenIn: contracts.AddressUSDCMainnet, TokenOut: contracts.AddressEURCMainnet, Recipients: []string{"0x3333333333333333333333333333333333333333"}, OutputAmounts: []*big.Int{big.NewInt(900_000)}, GrossInput: big.NewInt(1_000_000), MinTotalOut: big.NewInt(900_000), MinHopPriceX36: big.NewInt(1), Deadline: big.NewInt(1), ReferenceID: "reference"}
	call, err := payroll.EncodeCrossTokenPayroll(nil, input)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := payroll.DecodeCrossTokenPayrollCall(call.CallData())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.GrossInput.Cmp(input.GrossInput) != 0 || decoded.ReferenceID != input.ReferenceID || len(decoded.Recipients) != 1 {
		t.Fatalf("decoded=%#v", decoded)
	}
	malformed := call.CallData()
	malformed[0] ^= 0xff
	if _, err := payroll.DecodeCrossTokenPayrollCall(malformed); err == nil {
		t.Fatal("wrong selector accepted")
	}
}

func TestPayrollReferenceHashGlobalAcrossVariants(t *testing.T) {
	a, err := payroll.ReferenceHash("0x2222222222222222222222222222222222222222", "same-reference")
	if err != nil {
		t.Fatal(err)
	}
	b, err := payroll.ReferenceHash("0x2222222222222222222222222222222222222222", "same-reference")
	if err != nil || a != b {
		t.Fatal("reference replay domain is not stable")
	}
}
