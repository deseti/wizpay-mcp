package swap_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts/swap"
)

func TestVerifiedSwapABIContainsRequiredSurface(t *testing.T) {
	root := findModuleRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "contracts", "abi", "WizPaySwapExecutor.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("WizPaySwapExecutor.json parse failed: %v", err)
	}

	functions := map[string]bool{}
	events := map[string]bool{}
	for _, entry := range entries {
		switch entry["type"] {
		case "function":
			name, _ := entry["name"].(string)
			functions[name] = true
		case "event":
			name, _ := entry["name"].(string)
			events[name] = true
		}
	}
	for _, required := range []string{
		"executeSwap",
		"allowedRouters",
		"allowedTokens",
		"feeBps",
		"feeRecipient",
		"paused",
	} {
		if !functions[required] {
			t.Fatalf("required function %q missing from verified ABI", required)
		}
	}
	if !events["WizPaySwapExecuted"] {
		t.Fatal("required event WizPaySwapExecuted missing from verified ABI")
	}
	for _, admin := range swap.AdminFunctionNames {
		if !functions[admin] {
			t.Fatalf("expected admin function %q to exist in full ABI source", admin)
		}
	}
}

func TestRuntimeSwapABIHasNoAdminMethods(t *testing.T) {
	candidates := []string{
		"pause()",
		"unpause()",
		"rescueTokens(address,address,uint256)",
		"setFeeBps(uint256)",
		"setFeeRecipient(address)",
		"setRouterAllowed(address,bool)",
		"setTokenAllowed(address,bool)",
		"transferOwnership(address)",
		"renounceOwnership()",
	}
	for _, signature := range candidates {
		if _, err := swap.MethodBySignature(signature); err == nil {
			t.Fatalf("admin signature %q is exposed by runtime ABI", signature)
		}
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
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
