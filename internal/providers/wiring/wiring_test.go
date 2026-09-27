package wiring

import (
	"context"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/payroll"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/providers/arc"
	"github.com/deseti/wizpay-mcp/internal/providers/circle"
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

// configuredLookup models an attempted Circle Arc Mainnet activation. Track A
// must reject it before provider assembly.
func configuredLookup() func(string) (string, bool) {
	values := map[string]string{
		"WIZPAY_CIRCLE_ENABLED":    "true",
		"WIZPAY_CIRCLE_API_KEY":    "test-api-key",
		"WIZPAY_CIRCLE_BLOCKCHAIN": "ARC",
		"WIZPAY_ARC_ENABLED":       "true",
		"WIZPAY_ARC_CHAIN_ID":      arc.ChainIDMainnet,
		"WIZPAY_ARC_NETWORK":       arc.NetworkMainnet,
	}
	return func(key string) (string, bool) {
		value, found := values[key]
		return value, found
	}
}

func mainnetAttemptConfig(t *testing.T) Config {
	t.Helper()
	arcConfig, err := arc.LoadConfig(func(key string) (string, bool) {
		if key == "WIZPAY_ARC_ENABLED" {
			return "true", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	return Config{
		Circle: circle.Config{Enabled: true, BaseURL: "https://api.circle.com", Blockchain: circle.Blockchain("ARC"), ChainID: arc.ChainIDMainnet, Network: arc.NetworkMainnet, Timeout: 20 * time.Second},
		Arc:    arcConfig,
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

func TestLoadConfigRejectsCircleMainnetActivation(t *testing.T) {
	if _, err := LoadConfig(configuredLookup()); err == nil {
		t.Fatal("Circle Arc Mainnet activation must be rejected in Track A")
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
	config := mainnetAttemptConfig(t)
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
	if len(plane.ProviderFeatures(arc.ChainIDMainnet, arc.NetworkMainnet)) != 0 {
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

func TestBuildNeverActivatesCircleMainnetInTrackA(t *testing.T) {
	config := mainnetAttemptConfig(t)
	plane, err := Build(config, fullDependencies())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if plane.Adapter != nil || plane.Verifier != nil || plane.DomainVerifier != nil {
		t.Fatalf("Circle Mainnet attempt activated execution plane: %#v", plane)
	}
	features := plane.ProviderFeatures(arc.ChainIDMainnet, arc.NetworkMainnet)
	if len(features) != 0 {
		t.Fatalf("unreviewed provider must expose no Mainnet execution features, got %v", features)
	}
	// A different chain must expose nothing: features are chain-scoped.
	if len(plane.ProviderFeatures("1", arc.NetworkMainnet)) != 0 {
		t.Fatalf("features must not leak onto an unsupported chain")
	}
}

func TestBuildLeavesDomainVerifierUnconfiguredWithIncompleteTypedDependencies(t *testing.T) {
	config := mainnetAttemptConfig(t)
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
			if plane.Adapter != nil || plane.Verifier != nil || plane.DomainVerifier != nil {
				t.Fatalf("missing %s unexpectedly activated provider plane", test.name)
			}
		})
	}
}
