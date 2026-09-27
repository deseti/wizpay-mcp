package contracts_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"math/big"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	"github.com/deseti/wizpay-mcp/internal/contracts/swap"
)

func TestEncodedCallFieldsAreUnexported(t *testing.T) {
	typ := reflect.TypeOf(contracts.EncodedCall{})
	if typ.NumField() == 0 {
		t.Fatal("EncodedCall has no fields")
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath == "" {
			t.Fatalf("EncodedCall field %q is exported; arbitrary construction would be possible", field.Name)
		}
	}
}

func TestEncodedCallNoPublicStructLiteralFieldsInSource(t *testing.T) {
	root := findModuleRoot(t)
	path := filepath.Join(root, "internal", "contracts", "deployment.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "EncodedCall" {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				t.Fatal("EncodedCall is not a struct")
			}
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					if name.IsExported() {
						t.Fatalf("EncodedCall field %s is exported in source", name.Name)
					}
				}
			}
			return
		}
	}
	t.Fatal("EncodedCall type not found")
}

func TestNewEncodedCallRejectsAdminFunction(t *testing.T) {
	deployment := contracts.DefaultDeployments()[0]
	sel := contracts.Selector4("pause()")
	callData := append(sel[:], make([]byte, 32)...)
	_, err := contracts.NewEncodedCall(deployment, "pause()", sel, callData)
	if err == nil {
		t.Fatal("admin function construction must fail")
	}
}

func TestNewEncodedCallRejectsMismatchedSelector(t *testing.T) {
	deployment := contracts.DefaultDeployments()[0]
	sig := payroll.SigExecuteSameTokenPayroll
	bad := [4]byte{0xde, 0xad, 0xbe, 0xef}
	callData := append(bad[:], make([]byte, 32)...)
	_, err := contracts.NewEncodedCall(deployment, sig, bad, callData)
	if err == nil {
		t.Fatal("mismatched selector must fail")
	}
}

func TestNewEncodedCallUsesDeploymentAddressOnly(t *testing.T) {
	call, err := payroll.EncodeSameTokenPayroll(nil, payroll.SameTokenPayrollInput{
		Token: contracts.AddressUSDCMainnet, Recipients: []string{"0x3333333333333333333333333333333333333333"},
		Amounts: []*big.Int{big.NewInt(1)}, ReferenceID: "payroll-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !contracts.AddressesEqual(call.To(), contracts.AddressWizPayPayroll) {
		t.Fatalf("To = %q", call.To())
	}
	// Clone preserves values and isolates call data.
	cloned := call.Clone()
	if !bytes.Equal(cloned.CallData(), call.CallData()) {
		t.Fatal("clone call data mismatch")
	}
	mutated := cloned.CallData()
	mutated[0] ^= 0xff
	if bytes.Equal(call.CallData(), mutated) {
		t.Fatal("mutating clone CallData affected original")
	}
}

func TestPayrollAndSwapEncodersConstructSealedEncodedCall(t *testing.T) {
	payrollCall, err := payroll.EncodeSameTokenPayroll(contracts.DefaultRegistry(), payroll.SameTokenPayrollInput{
		Token: contracts.AddressUSDCMainnet, Recipients: []string{"0x3333333333333333333333333333333333333333"},
		Amounts: []*big.Int{big.NewInt(10)}, ReferenceID: "payroll-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if payrollCall.ContractID() != contracts.ContractWizPayPayroll {
		t.Fatalf("payroll contract id = %q", payrollCall.ContractID())
	}

	swapCall, err := swap.EncodeExecuteSwap(contracts.DefaultRegistry(), swap.ExecuteSwapInput{
		TokenIn:        contracts.AddressEURCMainnet,
		TokenOut:       contracts.AddressUSDCMainnet,
		AmountIn:       big.NewInt(10),
		MinAmountOut:   big.NewInt(1),
		MinHopPriceX36: big.NewInt(1),
		Deadline:       big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if swapCall.ContractID() != contracts.ContractWizPaySwapExecutor {
		t.Fatalf("swap contract id = %q", swapCall.ContractID())
	}
}
