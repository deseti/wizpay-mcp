package contracts_test

import (
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func TestDefaultRegistryPayrollAndSwap(t *testing.T) {
	registry := contracts.DefaultRegistry()

	payroll, err := registry.GetVersion(contracts.ContractWizPayPayroll, contracts.RegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	if payroll.Name != "WizPay" {
		t.Fatalf("payroll name = %q", payroll.Name)
	}
	if !contracts.AddressesEqual(payroll.Address, contracts.AddressWizPayPayroll) {
		t.Fatalf("payroll address = %q", payroll.Address)
	}
	if payroll.ChainID != contracts.ChainIDArcTestnet || payroll.Network != contracts.NetworkArcTestnet {
		t.Fatalf("payroll chain/network = %q/%q", payroll.ChainID, payroll.Network)
	}

	swap, err := registry.GetVersion(contracts.ContractWizPaySwapExecutor, contracts.RegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	if swap.Name != "WizPaySwapExecutor" {
		t.Fatalf("swap name = %q", swap.Name)
	}
	if !contracts.AddressesEqual(swap.Address, contracts.AddressWizPaySwapExecutor) {
		t.Fatalf("swap address = %q", swap.Address)
	}
}

func TestRegistryIdempotentAndConflict(t *testing.T) {
	registry := contracts.NewRegistry()
	payroll := contracts.DefaultDeployments()[0]
	if err := registry.Register(payroll); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(payroll); err != nil {
		t.Fatalf("identical re-registration should be idempotent: %v", err)
	}
	changed := payroll
	changed.Notes = "different"
	if err := registry.Register(changed); !hasCode(err, apperrors.CodeContractConflict) {
		t.Fatalf("conflicting registration error = %v", err)
	}
}

func TestRegistryListOrderingAndDefensiveCopy(t *testing.T) {
	registry := contracts.DefaultRegistry()
	list := registry.List()
	if len(list) != 2 {
		t.Fatalf("list length = %d", len(list))
	}
	if list[0].ID != contracts.ContractWizPayPayroll || list[1].ID != contracts.ContractWizPaySwapExecutor {
		t.Fatalf("list ordering = %#v", list)
	}
	list[0].Address = "0x0000000000000000000000000000000000000001"
	list[0].ExecutionFunctions[0] = "tampered()"
	again, err := registry.GetVersion(contracts.ContractWizPayPayroll, contracts.RegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	if again.Address == list[0].Address || again.ExecutionFunctions[0] == "tampered()" {
		t.Fatal("registry state was mutated through returned slices")
	}
}

func TestRegistryExactLookupAndWrongChain(t *testing.T) {
	registry := contracts.DefaultRegistry()
	_, err := registry.GetVersion(contracts.ContractWizPayPayroll, 99)
	if !hasCode(err, apperrors.CodeContractNotFound) {
		t.Fatalf("missing version error = %v", err)
	}
	_, err = registry.Require(contracts.ContractWizPayPayroll, contracts.RegistryVersion, "1", contracts.NetworkArcTestnet)
	if !hasCode(err, apperrors.CodeValidationError) {
		t.Fatalf("wrong chain error = %v", err)
	}
}

func TestRegistryRejectsMalformedAddressAndAmbiguity(t *testing.T) {
	registry := contracts.NewRegistry()
	bad := contracts.DefaultDeployments()[0]
	bad.Address = "not-an-address"
	if err := registry.Register(bad); !hasCode(err, apperrors.CodeValidationError) {
		t.Fatalf("malformed address error = %v", err)
	}

	if err := registry.Register(contracts.DefaultDeployments()[0]); err != nil {
		t.Fatal(err)
	}
	// Attempt to register swap with payroll's address should fail validation
	// because swap ID requires the swap address; craft a conflicting location
	// by registering payroll then a mutated second version with same address.
	conflict := contracts.DefaultDeployments()[0]
	conflict.RegistryVersion = 2
	// Same address different version is allowed only if location index permits
	// same ID; version 2 with same ID+address is fine for location map overwrite
	// of same ID. Ambiguity is different ID at same address.
	swapConflict := contracts.DefaultDeployments()[1]
	swapConflict.Address = contracts.AddressWizPayPayroll
	if err := registry.Register(swapConflict); !hasCode(err, apperrors.CodeValidationError) {
		t.Fatalf("address mismatch for swap ID error = %v", err)
	}
}

func TestRegistryLookupByAddress(t *testing.T) {
	registry := contracts.DefaultRegistry()
	got, err := registry.LookupByAddress(contracts.ChainIDArcTestnet, contracts.AddressWizPaySwapExecutor)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != contracts.ContractWizPaySwapExecutor {
		t.Fatalf("lookup ID = %q", got.ID)
	}
	_, err = registry.LookupByAddress(contracts.ChainIDArcTestnet, "0x00000000000000000000000000000000000000aa")
	if !hasCode(err, apperrors.CodeContractNotFound) {
		t.Fatalf("unknown address error = %v", err)
	}
}

func TestNoFXEngineDeploymentRegistered(t *testing.T) {
	for _, deployment := range contracts.DefaultDeployments() {
		if deployment.ID == "FX_ENGINE" || deployment.Name == "FXEngine" || deployment.Name == "fxEngine" {
			t.Fatalf("unexpected FX Engine deployment: %#v", deployment)
		}
		for _, fn := range deployment.ExecutionFunctions {
			if fn == "updateFXEngine(address)" {
				t.Fatalf("admin updateFXEngine must not be execution surface: %q", fn)
			}
		}
	}
	list := contracts.DefaultRegistry().List()
	if len(list) != 2 {
		t.Fatalf("expected only payroll and swap, got %d", len(list))
	}
}

func hasCode(err error, code apperrors.Code) bool {
	if err == nil {
		return false
	}
	public := apperrors.ToPublic(err)
	return public.Code == code
}
