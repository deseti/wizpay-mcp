package payroll_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/deseti/wizpay-mcp/internal/contracts/payroll"
)

func TestVerifiedPayrollMainnetABIContainsExactSurface(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(moduleRoot(t), "contracts", "abi", "WizPayPayrollMainnet.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct{ Type, Name string }
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	parsed, err := abi.JSON(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	for name, signature := range map[string]string{
		"executeSameTokenPayroll":  payroll.SigExecuteSameTokenPayroll,
		"executeCrossTokenPayroll": payroll.SigExecuteCrossTokenPayroll,
	} {
		method, ok := parsed.Methods[name]
		if !ok || method.Sig != signature {
			t.Fatalf("full ABI %s signature = %q, want %q", name, method.Sig, signature)
		}
	}
	for name, signature := range map[string]string{
		"PayrollBatchExecuted":     payroll.SigPayrollBatchExecuted,
		"PayrollPayment":           payroll.SigPayrollPayment,
		"PayrollReferenceConsumed": payroll.SigPayrollReferenceConsumed,
		"PayrollSurplusRefunded":   payroll.SigPayrollSurplusRefunded,
		"PayrollSwapExecuted":      payroll.SigPayrollSwapExecuted,
	} {
		event, ok := parsed.Events[name]
		if !ok || event.Sig != signature {
			t.Fatalf("full ABI %s signature = %q, want %q", name, event.Sig, signature)
		}
	}
	functions, events := map[string]bool{}, map[string]bool{}
	for _, entry := range entries {
		if entry.Type == "function" {
			functions[entry.Name] = true
		}
		if entry.Type == "event" {
			events[entry.Name] = true
		}
	}
	for _, name := range []string{"executeSameTokenPayroll", "executeCrossTokenPayroll", "USDC", "EURC", "universalRouter", "permit2", "poolManager"} {
		if !functions[name] {
			t.Fatalf("missing function %s", name)
		}
	}
	for _, name := range []string{"PayrollBatchExecuted", "PayrollPayment", "PayrollReferenceConsumed", "PayrollSurplusRefunded", "PayrollSwapExecuted"} {
		if !events[name] {
			t.Fatalf("missing event %s", name)
		}
	}
	for _, old := range []string{"batchRouteAndPay", "routeAndPay"} {
		if functions[old] {
			t.Fatalf("stale Testnet function %s", old)
		}
	}
	for _, admin := range payroll.AdminFunctionNames {
		if !functions[admin] {
			t.Fatalf("full ABI missing admin function %s", admin)
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
