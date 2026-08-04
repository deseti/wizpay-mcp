package payroll_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts/payroll"
)

func TestVerifiedWizPayABIContainsRequiredSurface(t *testing.T) {
	root := findModuleRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "contracts", "abi", "WizPay.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("WizPay.json parse failed: %v", err)
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
		"batchRouteAndPay",
		"routeAndPay",
		"getBatchEstimatedOutputs",
		"getEstimatedOutput",
		"paused",
		"whitelistEnabled",
		"whitelistedTokens",
		"feeBps",
	} {
		if !functions[required] {
			t.Fatalf("required function %q missing from verified ABI", required)
		}
	}
	for _, required := range []string{"BatchPaymentRouted", "PaymentRouted"} {
		if !events[required] {
			t.Fatalf("required event %q missing from verified ABI", required)
		}
	}
	// Admin functions may exist in source ABI...
	for _, admin := range payroll.AdminFunctionNames {
		if !functions[admin] {
			t.Fatalf("expected admin function %q to exist in full ABI source", admin)
		}
	}
	// ...but must not be present on the runtime allowlisted fragment.
	for _, admin := range payroll.AdminFunctionNames {
		if _, err := payroll.MethodBySignature(admin + "()"); err == nil {
			t.Fatalf("admin function %q unexpectedly resolvable on runtime ABI fragment", admin)
		}
	}
}

func TestRuntimeABIHasNoAdminMethods(t *testing.T) {
	// Ensure none of the admin names are available as MethodBySignature with
	// any common signature shape used in the full ABI.
	candidates := []string{
		"emergencyWithdraw(address,uint256)",
		"pause()",
		"unpause()",
		"setTokenWhitelist(address,bool)",
		"batchSetTokenWhitelist(address[],bool)",
		"setWhitelistEnabled(bool)",
		"updateFXEngine(address)",
		"updateFee(uint256)",
		"updateFeeCollector(address)",
		"transferOwnership(address)",
		"renounceOwnership()",
	}
	for _, signature := range candidates {
		if _, err := payroll.MethodBySignature(signature); err == nil {
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
