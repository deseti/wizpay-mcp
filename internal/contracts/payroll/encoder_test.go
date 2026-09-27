package payroll_test

import (
	"bytes"
	"math/big"
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
