package swap_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/deseti/wizpay-mcp/internal/contracts/swap"
)

func TestVerifiedSwapMainnetABIContainsExactSurface(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(findModuleRoot(t), "contracts", "abi", "WizPaySwapExecutorMainnet.json"))
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
	method, ok := parsed.Methods["executeSwap"]
	if !ok || method.Sig != swap.SigExecuteSwap {
		t.Fatalf("full ABI executeSwap signature = %q, want %q", method.Sig, swap.SigExecuteSwap)
	}
	event, ok := parsed.Events["WizPayMainnetSwapExecuted"]
	if !ok || event.Sig != swap.SigWizPayMainnetSwapExecuted {
		t.Fatalf("full ABI swap event signature = %q, want %q", event.Sig, swap.SigWizPayMainnetSwapExecuted)
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
	for _, name := range []string{"executeSwap", "USDC", "EURC", "universalRouter", "permit2", "poolFee", "poolTickSpacing"} {
		if !functions[name] {
			t.Fatalf("missing function %s", name)
		}
	}
	if !events["WizPayMainnetSwapExecuted"] || events["WizPaySwapExecuted"] {
		t.Fatal("swap event surface mismatch")
	}
	for _, old := range []string{"allowedRouters", "allowedTokens"} {
		if functions[old] {
			t.Fatalf("stale Testnet function %s", old)
		}
	}
	for _, admin := range swap.AdminFunctionNames {
		if !functions[admin] {
			t.Fatalf("full ABI missing admin function %s", admin)
		}
	}
}

func findModuleRoot(t *testing.T) string {
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
