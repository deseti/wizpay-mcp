// Package wiring assembles the provider execution plane from configuration.
//
// It exists as a separate package so the provider-neutral core, the Circle
// boundary, and the Arc boundary never import one another. Assembly is
// declarative and fail-closed: a provider whose configuration or dependencies
// are absent is registered as unconfigured rather than constructed in a
// degraded state.
package wiring

import (
	"fmt"
	"net/http"
	"time"

	"github.com/deseti/wizpay-mcp/internal/capabilities"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/providers/arc"
	"github.com/deseti/wizpay-mcp/internal/providers/circle"
)

// Config is the resolved provider-plane configuration. Secrets live inside the
// provider configs, which redact themselves; this struct never carries a raw
// credential of its own.
type Config struct {
	Circle circle.Config
	Arc    arc.Config
}

// LoadConfig reads both provider configurations from the environment.
func LoadConfig(lookup func(string) (string, bool)) (Config, error) {
	circleConfig, err := circle.LoadConfig(lookup)
	if err != nil {
		return Config{}, err
	}
	arcConfig, err := arc.LoadConfig(lookup)
	if err != nil {
		return Config{}, err
	}
	return Config{Circle: circleConfig, Arc: arcConfig}, nil
}

// Dependencies are the ports the provider plane cannot supply itself.
//
// Planner is deliberately required from outside: turning an approved intent
// into a concrete transfer is capability logic, which Phase 11 does not
// implement. Without it the Circle provider registers as unconfigured.
type Dependencies struct {
	Planner       providers.Planner
	Authorization providers.AuthorizationSource
	References    circle.ReferenceStore
	HTTPClient    *http.Client
	Now           func() time.Time
}

// Plane is the assembled provider execution layer.
type Plane struct {
	// Registry answers which provider can serve a feature on a chain.
	Registry *providers.Registry
	// Verifier is the Phase 9 runtime.Verifier: provider reconciliation
	// combined with Arc receipt verification. It is nil when Arc is not
	// configured, because no execution may be verified without chain evidence.
	Verifier *providers.Verifier
	// Adapter is the Phase 9 execution.Adapter, or nil when no provider is
	// configured. A nil adapter must never be replaced with a permissive stub.
	Adapter *circle.Adapter
}

// Build assembles the provider plane. Every provider is registered so the
// registry can report why one is unavailable; only fully configured providers
// carry an adapter.
func Build(config Config, dependencies Dependencies) (Plane, error) {
	if dependencies.Now == nil {
		return Plane{}, fmt.Errorf("provider plane requires a clock")
	}
	registry := providers.NewRegistry()

	var adapter *circle.Adapter
	configured := config.Circle.Configured() && config.Arc.Configured() &&
		dependencies.Planner != nil && dependencies.Authorization != nil && dependencies.References != nil
	if configured {
		built, err := circle.NewAdapter(config.Circle, dependencies.HTTPClient, dependencies.Planner,
			dependencies.Authorization, dependencies.References, dependencies.Now)
		if err != nil {
			return Plane{}, err
		}
		adapter = built
	}

	descriptor := providers.Descriptor{
		ID:      providers.ProviderCircleUserControlled,
		Version: 1,
		// The declared features are exactly what this provider boundary
		// implements: a user-controlled wallet and a token transfer. Contract
		// execution, swap, bridge, and ANS features are deliberately not
		// claimed, so Phase 10 keeps those capabilities unavailable.
		Features:   []capabilities.ProviderFeature{capabilities.FeatureUserControlledWallet, capabilities.FeatureTokenTransfer},
		ChainIDs:   []string{arc.ChainIDTestnet},
		Networks:   []string{arc.NetworkTestnet},
		Configured: configured,
	}
	if adapter != nil {
		descriptor.Adapter = adapter
	}
	if err := registry.Register(descriptor); err != nil {
		return Plane{}, err
	}

	plane := Plane{Registry: registry, Adapter: adapter}
	if !config.Arc.Configured() || adapter == nil {
		// Without a chain reader or a reconciliation source there is no way to
		// obtain on-chain evidence, and a provider status alone may never
		// verify an execution.
		return plane, nil
	}
	client, err := arc.NewClient(config.Arc, dependencies.HTTPClient)
	if err != nil {
		return Plane{}, err
	}
	chain, err := arc.NewVerifier(config.Arc, client)
	if err != nil {
		return Plane{}, err
	}
	verifier, err := providers.NewVerifier(chain, adapter,
		providers.VerifierConfig{MinConfirmations: config.Arc.MinConfirmations}, dependencies.Now)
	if err != nil {
		return Plane{}, err
	}
	plane.Verifier = verifier
	return plane, nil
}

// ProviderFeatures reports the features actually available on a chain and
// network. This is the value Phase 10 consumes as
// capabilities.AvailabilityRequest.ProviderFeatures.
//
// It reports provider reachability only. A capability still has to be enabled
// in the capability registry on its own terms; a configured provider never
// enables one by itself.
func (p Plane) ProviderFeatures(chainID, network string) []capabilities.ProviderFeature {
	if p.Registry == nil {
		return nil
	}
	return p.Registry.Features(chainID, network)
}
