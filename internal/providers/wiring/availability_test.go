package wiring

import (
	"testing"

	"github.com/deseti/wizpay-mcp/internal/capabilities"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers/arc"
)

// enabledPayrollDescriptor is an ENABLED capability that requires exactly the
// two features a configured Circle+Arc plane supplies. The shipped defaults are
// all disabled, so a bespoke enabled descriptor is needed to exercise the
// feature-gating path rather than the status gate.
func enabledPayrollDescriptor() capabilities.Descriptor {
	return capabilities.Descriptor{
		ID:                capabilities.CapabilityPayroll,
		Version:           1,
		Status:            capabilities.StatusEnabled,
		IntentType:        intents.TypePayroll,
		SupportedChains:   []string{arc.ChainIDTestnet},
		SupportedNetworks: []string{arc.NetworkTestnet},
		ProviderFeatures: []capabilities.ProviderFeature{
			capabilities.FeatureUserControlledWallet,
			capabilities.FeatureTokenTransfer,
		},
		Description: "Payroll capability used for availability wiring tests.",
	}
}

func registryWith(t *testing.T, descriptor capabilities.Descriptor) *capabilities.Registry {
	t.Helper()
	registry := capabilities.NewRegistry()
	if err := registry.Register(descriptor); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return registry
}

func configuredPlane(t *testing.T) Plane {
	t.Helper()
	config, err := LoadConfig(configuredLookup())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	plane, err := Build(config, fullDependencies())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return plane
}

func unconfiguredPlane(t *testing.T) Plane {
	t.Helper()
	config, err := LoadConfig(emptyLookup())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	plane, err := Build(config, fullDependencies())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return plane
}

func TestNewAvailabilityRequiresRegistryAndPlane(t *testing.T) {
	if _, err := NewAvailability(nil, configuredPlane(t)); err == nil {
		t.Fatalf("a nil registry must be rejected")
	}
	if _, err := NewAvailability(capabilities.NewRegistry(), Plane{}); err == nil {
		t.Fatalf("an unassembled plane must be rejected")
	}
}

func TestResolveAvailableWhenPlaneSuppliesFeatures(t *testing.T) {
	availability, err := NewAvailability(registryWith(t, enabledPayrollDescriptor()), configuredPlane(t))
	if err != nil {
		t.Fatalf("NewAvailability: %v", err)
	}
	decision, err := availability.Resolve(capabilities.CapabilityPayroll, capabilities.AvailabilityRequest{
		ChainID: arc.ChainIDTestnet,
		Network: arc.NetworkTestnet,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !decision.Available || decision.Reason != capabilities.ReasonAvailable {
		t.Fatalf("expected available, got available=%v reason=%s", decision.Available, decision.Reason)
	}
}

func TestResolveUnavailableWhenPlaneUnconfigured(t *testing.T) {
	availability, err := NewAvailability(registryWith(t, enabledPayrollDescriptor()), unconfiguredPlane(t))
	if err != nil {
		t.Fatalf("NewAvailability: %v", err)
	}
	decision, err := availability.Resolve(capabilities.CapabilityPayroll, capabilities.AvailabilityRequest{
		ChainID: arc.ChainIDTestnet,
		Network: arc.NetworkTestnet,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if decision.Available {
		t.Fatalf("a capability requiring provider features must be unavailable when no provider is configured")
	}
	if decision.Reason != capabilities.ReasonMissingProviderFeature {
		t.Fatalf("expected MISSING_PROVIDER_FEATURE, got %s", decision.Reason)
	}
}

func TestResolveDiscardsCallerSuppliedFeatures(t *testing.T) {
	// A caller must not be able to assert a provider feature into existence: the
	// plane is the only source of truth. With an unconfigured plane the request's
	// own features are ignored and the capability stays unavailable.
	availability, err := NewAvailability(registryWith(t, enabledPayrollDescriptor()), unconfiguredPlane(t))
	if err != nil {
		t.Fatalf("NewAvailability: %v", err)
	}
	decision, err := availability.Resolve(capabilities.CapabilityPayroll, capabilities.AvailabilityRequest{
		ChainID: arc.ChainIDTestnet,
		Network: arc.NetworkTestnet,
		ProviderFeatures: []capabilities.ProviderFeature{
			capabilities.FeatureUserControlledWallet,
			capabilities.FeatureTokenTransfer,
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if decision.Available {
		t.Fatalf("caller-supplied features must be discarded, keeping the capability unavailable")
	}
	if decision.Reason != capabilities.ReasonMissingProviderFeature {
		t.Fatalf("expected MISSING_PROVIDER_FEATURE, got %s", decision.Reason)
	}
}
