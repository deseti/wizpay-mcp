package wiring

import (
	"context"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/capabilities"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/payroll"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/providers/arc"
	"github.com/deseti/wizpay-mcp/internal/swap"
)

// fakePlanner is never invoked during assembly; its presence alone lets the
// Circle adapter be constructed.
type fakePlanner struct{}

func (fakePlanner) Plan(context.Context, execution.Request) (providers.Plan, error) {
	return providers.Plan{}, nil
}

// fakeReferenceStore satisfies circle.ReferenceStore for assembly tests.
type fakeReferenceStore struct{}

func (fakeReferenceStore) LatestReference(context.Context, string) (providers.Reference, bool, error) {
	return providers.Reference{}, false, nil
}

func fixedClock() func() time.Time {
	instant := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return instant }
}

// configuredLookup returns an environment in which both providers are fully
// configured for Arc Testnet.
func configuredLookup() func(string) (string, bool) {
	values := map[string]string{
		"WIZPAY_CIRCLE_ENABLED": "true",
		"WIZPAY_CIRCLE_API_KEY": "test-api-key",
		"WIZPAY_ARC_ENABLED":    "true",
		"WIZPAY_ARC_CHAIN_ID":   arc.ChainIDTestnet,
		"WIZPAY_ARC_NETWORK":    arc.NetworkTestnet,
	}
	return func(key string) (string, bool) {
		value, found := values[key]
		return value, found
	}
}

func emptyLookup() func(string) (string, bool) {
	return func(string) (string, bool) { return "", false }
}

func fullDependencies() Dependencies {
	payrollPlanner := payroll.NewPlanner(nil)
	swapPlanner := swap.NewPlanner(nil)
	payrollVerifier := payroll.NewVerifier(nil)
	swapVerifier := swap.NewVerifier(nil)
	return Dependencies{
		Planner:       fakePlanner{},
		Authorization: providers.ContextAuthorizationSource{},
		References:    fakeReferenceStore{},
		Intents:       &intentRepositoryStub{},
		Payroll:       &payrollPlanner,
		Swap:          &swapPlanner,
		PayrollV:      &payrollVerifier,
		SwapV:         &swapVerifier,
		Now:           fixedClock(),
	}
}

func TestLoadConfigDisabledByDefault(t *testing.T) {
	config, err := LoadConfig(emptyLookup())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if config.Circle.Configured() || config.Arc.Configured() {
		t.Fatalf("providers must be unconfigured by default")
	}
}

func TestBuildRequiresClock(t *testing.T) {
	config, err := LoadConfig(emptyLookup())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	deps := fullDependencies()
	deps.Now = nil
	if _, err := Build(config, deps); err == nil {
		t.Fatalf("Build must reject a missing clock")
	}
}

func TestBuildUnconfiguredWithoutPlanner(t *testing.T) {
	// Even with both providers configured, the absence of a planner must leave
	// the adapter and verifier nil: no execution may be driven.
	config, err := LoadConfig(configuredLookup())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !config.Circle.Configured() || !config.Arc.Configured() {
		t.Fatalf("expected both providers configured")
	}
	deps := fullDependencies()
	deps.Planner = nil
	plane, err := Build(config, deps)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if plane.Adapter != nil {
		t.Fatalf("adapter must be nil without a planner")
	}
	if plane.Verifier != nil {
		t.Fatalf("verifier must be nil without a configured adapter")
	}
	if len(plane.ProviderFeatures(arc.ChainIDTestnet, arc.NetworkTestnet)) != 0 {
		t.Fatalf("an unconfigured provider must expose no features")
	}
}

func TestBuildUnconfiguredWhenProvidersDisabled(t *testing.T) {
	config, err := LoadConfig(emptyLookup())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	plane, err := Build(config, fullDependencies())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if plane.Adapter != nil || plane.Verifier != nil {
		t.Fatalf("disabled providers must produce no adapter or verifier")
	}
	if plane.Registry == nil {
		t.Fatalf("registry must always be present so unavailability can be explained")
	}
}

func TestBuildConfigured(t *testing.T) {
	config, err := LoadConfig(configuredLookup())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	plane, err := Build(config, fullDependencies())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if plane.Adapter == nil {
		t.Fatalf("configured providers must produce an adapter")
	}
	if plane.Verifier == nil {
		t.Fatalf("configured providers must produce a verifier")
	}
	if plane.DomainVerifier == nil {
		t.Fatalf("configured providers must produce a typed domain verifier")
	}
	features := plane.ProviderFeatures(arc.ChainIDTestnet, arc.NetworkTestnet)
	if !containsFeature(features, capabilities.FeatureUserControlledWallet) || !containsFeature(features, capabilities.FeatureContractExecution) || !containsFeature(features, capabilities.FeatureSwapExecution) {
		t.Fatalf("configured provider must expose its declared features, got %v", features)
	}
	// A different chain must expose nothing: features are chain-scoped.
	if len(plane.ProviderFeatures("1", arc.NetworkTestnet)) != 0 {
		t.Fatalf("features must not leak onto an unsupported chain")
	}
}

func TestBuildLeavesDomainVerifierUnconfiguredWithIncompleteTypedDependencies(t *testing.T) {
	config, err := LoadConfig(configuredLookup())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		drop func(*Dependencies)
	}{
		{name: "intent repository", drop: func(deps *Dependencies) { deps.Intents = nil }},
		{name: "payroll planner", drop: func(deps *Dependencies) { deps.Payroll = nil }},
		{name: "swap planner", drop: func(deps *Dependencies) { deps.Swap = nil }},
		{name: "payroll verifier", drop: func(deps *Dependencies) { deps.PayrollV = nil }},
		{name: "swap verifier", drop: func(deps *Dependencies) { deps.SwapV = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := fullDependencies()
			test.drop(&deps)
			plane, err := Build(config, deps)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if plane.Verifier == nil {
				t.Fatal("generic verifier should remain assembled")
			}
			if plane.DomainVerifier != nil {
				t.Fatalf("missing %s produced a domain verifier", test.name)
			}
		})
	}
}

func containsFeature(features []capabilities.ProviderFeature, target capabilities.ProviderFeature) bool {
	for _, feature := range features {
		if feature == target {
			return true
		}
	}
	return false
}
