package wiring

import (
	"testing"

	"github.com/deseti/wizpay-mcp/internal/capabilities"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/providers/arc"
)

// enabledDescriptor is an ENABLED capability used only to exercise provider
// feature resolution. Shipped defaults remain disabled.
func enabledDescriptor(id capabilities.CapabilityID, intentType intents.Type, features ...capabilities.ProviderFeature) capabilities.Descriptor {
	return capabilities.Descriptor{ID: id, Version: 1, Status: capabilities.StatusEnabled, IntentType: intentType, SupportedChains: []string{arc.ChainIDTestnet}, SupportedNetworks: []string{arc.NetworkTestnet}, ProviderFeatures: features, Description: "Capability used for availability wiring tests."}
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
		t.Fatal("a nil registry must be rejected")
	}
	if _, err := NewAvailability(capabilities.NewRegistry(), Plane{}); err == nil {
		t.Fatal("an unassembled plane must be rejected")
	}
}

func TestResolveAvailableWhenPlaneSuppliesFeatures(t *testing.T) {
	availability, err := NewAvailability(registryWith(t, enabledDescriptor(capabilities.CapabilityPayroll, intents.TypePayroll, capabilities.FeatureUserControlledWallet, capabilities.FeatureContractExecution)), configuredPlane(t))
	if err != nil {
		t.Fatalf("NewAvailability: %v", err)
	}
	decision, err := availability.Resolve(capabilities.CapabilityPayroll, capabilities.AvailabilityRequest{ChainID: arc.ChainIDTestnet, Network: arc.NetworkTestnet})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !decision.Available || decision.Reason != capabilities.ReasonAvailable {
		t.Fatalf("expected available, got available=%v reason=%s", decision.Available, decision.Reason)
	}
}

func TestTypedCircleDescriptorSatisfiesPayrollAndSwap(t *testing.T) {
	plane := configuredPlane(t)
	for _, test := range []struct {
		id         capabilities.CapabilityID
		intentType intents.Type
		features   []capabilities.ProviderFeature
	}{
		{capabilities.CapabilityPayroll, intents.TypePayroll, []capabilities.ProviderFeature{capabilities.FeatureUserControlledWallet, capabilities.FeatureContractExecution}},
		{capabilities.CapabilitySwap, intents.TypeSwap, []capabilities.ProviderFeature{capabilities.FeatureUserControlledWallet, capabilities.FeatureContractExecution, capabilities.FeatureSwapExecution}},
	} {
		t.Run(string(test.id), func(t *testing.T) {
			availability, err := NewAvailability(registryWith(t, enabledDescriptor(test.id, test.intentType, test.features...)), plane)
			if err != nil {
				t.Fatal(err)
			}
			decision, err := availability.Resolve(test.id, capabilities.AvailabilityRequest{ChainID: arc.ChainIDTestnet, Network: arc.NetworkTestnet})
			if err != nil || !decision.Available {
				t.Fatalf("typed provider should satisfy %s: decision=%#v error=%v", test.id, decision, err)
			}
		})
	}
}

func TestTypedCapabilitiesRequireTheirExecutionFeatures(t *testing.T) {
	for _, test := range []struct {
		name    string
		id      capabilities.CapabilityID
		intent  intents.Type
		missing capabilities.ProviderFeature
	}{
		{"payroll without contract execution", capabilities.CapabilityPayroll, intents.TypePayroll, capabilities.FeatureContractExecution},
		{"swap without contract execution", capabilities.CapabilitySwap, intents.TypeSwap, capabilities.FeatureContractExecution},
		{"swap without swap execution", capabilities.CapabilitySwap, intents.TypeSwap, capabilities.FeatureSwapExecution},
	} {
		t.Run(test.name, func(t *testing.T) {
			plane := configuredPlane(t)
			provider, err := plane.Registry.GetVersion(providers.ProviderCircleUserControlled, 1)
			if err != nil {
				t.Fatal(err)
			}
			features := []capabilities.ProviderFeature{capabilities.FeatureUserControlledWallet, capabilities.FeatureContractExecution, capabilities.FeatureSwapExecution}
			filtered := make([]capabilities.ProviderFeature, 0, len(features)-1)
			for _, feature := range features {
				if feature != test.missing {
					filtered = append(filtered, feature)
				}
			}
			provider.Features = filtered
			providerRegistry := providers.NewRegistry()
			if err := providerRegistry.Register(provider); err != nil {
				t.Fatal(err)
			}
			plane.Registry = providerRegistry
			var required []capabilities.ProviderFeature
			if test.id == capabilities.CapabilityPayroll {
				required = []capabilities.ProviderFeature{capabilities.FeatureUserControlledWallet, capabilities.FeatureContractExecution}
			} else {
				required = features
			}
			availability, err := NewAvailability(registryWith(t, enabledDescriptor(test.id, test.intent, required...)), plane)
			if err != nil {
				t.Fatal(err)
			}
			decision, err := availability.Resolve(test.id, capabilities.AvailabilityRequest{ChainID: arc.ChainIDTestnet, Network: arc.NetworkTestnet})
			if err != nil || decision.Available || decision.Reason != capabilities.ReasonMissingProviderFeature {
				t.Fatalf("expected missing provider feature: decision=%#v error=%v", decision, err)
			}
		})
	}
}

func TestResolveUnavailableWhenPlaneUnconfigured(t *testing.T) {
	availability, err := NewAvailability(registryWith(t, enabledDescriptor(capabilities.CapabilityPayroll, intents.TypePayroll, capabilities.FeatureUserControlledWallet, capabilities.FeatureContractExecution)), unconfiguredPlane(t))
	if err != nil {
		t.Fatalf("NewAvailability: %v", err)
	}
	decision, err := availability.Resolve(capabilities.CapabilityPayroll, capabilities.AvailabilityRequest{ChainID: arc.ChainIDTestnet, Network: arc.NetworkTestnet})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if decision.Available || decision.Reason != capabilities.ReasonMissingProviderFeature {
		t.Fatalf("expected missing provider feature, got decision=%#v", decision)
	}
}

func TestResolveDiscardsCallerSuppliedFeatures(t *testing.T) {
	availability, err := NewAvailability(registryWith(t, enabledDescriptor(capabilities.CapabilityPayroll, intents.TypePayroll, capabilities.FeatureUserControlledWallet, capabilities.FeatureContractExecution)), unconfiguredPlane(t))
	if err != nil {
		t.Fatalf("NewAvailability: %v", err)
	}
	decision, err := availability.Resolve(capabilities.CapabilityPayroll, capabilities.AvailabilityRequest{ChainID: arc.ChainIDTestnet, Network: arc.NetworkTestnet, ProviderFeatures: []capabilities.ProviderFeature{capabilities.FeatureUserControlledWallet, capabilities.FeatureContractExecution}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if decision.Available || decision.Reason != capabilities.ReasonMissingProviderFeature {
		t.Fatalf("caller-supplied features must be discarded: decision=%#v", decision)
	}
}

func TestBridgeAndANSRemainUnavailable(t *testing.T) {
	plane := configuredPlane(t)
	for _, test := range []struct {
		id capabilities.CapabilityID
		in intents.Type
	}{{capabilities.CapabilityBridge, intents.TypeBridge}, {capabilities.CapabilityANS, intents.TypeANSRegistration}} {
		availability, err := NewAvailability(registryWith(t, enabledDescriptor(test.id, test.in, capabilities.FeatureUserControlledWallet, capabilities.FeatureBridgeExecution, capabilities.FeatureANSRegistration)), plane)
		if err != nil {
			t.Fatal(err)
		}
		decision, err := availability.Resolve(test.id, capabilities.AvailabilityRequest{ChainID: arc.ChainIDTestnet, Network: arc.NetworkTestnet})
		if err != nil || decision.Available {
			t.Fatalf("%s should remain unavailable: decision=%#v error=%v", test.id, decision, err)
		}
	}
}
