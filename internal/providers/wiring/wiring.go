// Package wiring assembles the provider execution plane from configuration.
//
// It exists as a separate package so the provider-neutral core, the Circle
// boundary, and the Arc boundary never import one another. Assembly is
// declarative and fail-closed: a provider whose configuration or dependencies
// are absent is registered as unconfigured rather than constructed in a
// degraded state.
package wiring

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/deseti/wizpay-mcp/internal/capabilities"
	"github.com/deseti/wizpay-mcp/internal/execution/runtime"
	"github.com/deseti/wizpay-mcp/internal/payroll"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/providers/arc"
	"github.com/deseti/wizpay-mcp/internal/providers/circle"
	"github.com/deseti/wizpay-mcp/internal/providers/circuit"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/swap"
)

// Config is the resolved provider-plane configuration. Secrets live inside the
// provider configs, which redact themselves; this struct never carries a raw
// credential of its own.
type Config struct {
	Circle circle.Config
	Arc    arc.Config
	// Breaker is the shared infrastructure-breaker configuration for outbound
	// Circle and Arc calls. Zero value is replaced with circuit.DefaultConfig.
	Breaker circuit.Config
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
	return Config{
		Circle:  circleConfig,
		Arc:     arcConfig,
		Breaker: circuit.DefaultConfig(),
	}, nil
}

// Dependencies are the ports the provider plane cannot supply itself.
//
// Planner is deliberately required from outside: turning an approved intent
// into a concrete transfer is capability logic. Without it the Circle provider
// registers as unconfigured.
type Dependencies struct {
	Planner       providers.Planner
	Authorization providers.AuthorizationSource
	References    circle.ReferenceStore
	Intents       storage.IntentRepository
	Payroll       *payroll.Planner
	Swap          *swap.Planner
	PayrollV      *payroll.Verifier
	SwapV         *swap.Verifier
	HTTPClient    *http.Client
	Now           func() time.Time
}

// Plane is the assembled provider execution layer.
type Plane struct {
	// Registry answers which provider can serve a feature on a chain.
	Registry *providers.Registry
	// Verifier performs generic provider reconciliation combined with Arc receipt
	// verification. It is nil when Arc is not
	// configured, because no execution may be verified without chain evidence.
	Verifier *providers.Verifier
	// DomainVerifier is the only verifier permitted to drive runtime completion
	// for typed Payroll/Swap contract executions.
	DomainVerifier runtime.Verifier
	// Adapter is the execution adapter, or nil when no provider is
	// configured. A nil adapter must never be replaced with a permissive stub.
	Adapter *circle.Adapter
	// HealthCheckers are optional non-financial readiness probes. Process
	// liveness must not depend on them.
	HealthCheckers []providers.HealthChecker
	// CircleBreaker and ArcBreaker expose safe breaker snapshots for health.
	CircleBreaker *circuit.Breaker
	ArcBreaker    *circuit.Breaker
}

// Build assembles the provider plane. Every provider is registered so the
// registry can report why one is unavailable; only fully configured providers
// carry an adapter.
func Build(config Config, dependencies Dependencies) (Plane, error) {
	if dependencies.Now == nil {
		return Plane{}, fmt.Errorf("provider plane requires a clock")
	}
	breakerConfig := config.Breaker
	if breakerConfig.FailureThreshold == 0 {
		breakerConfig = circuit.DefaultConfig()
	}
	circleBreaker, err := circuit.New(breakerConfig, dependencies.Now)
	if err != nil {
		return Plane{}, err
	}
	arcBreaker, err := circuit.New(breakerConfig, dependencies.Now)
	if err != nil {
		return Plane{}, err
	}

	registry := providers.NewRegistry()

	var adapter *circle.Adapter
	configured := config.Circle.Configured() && config.Arc.Configured() &&
		dependencies.Planner != nil && dependencies.Authorization != nil && dependencies.References != nil
	if configured {
		built, err := circle.NewAdapterWithBreaker(config.Circle, dependencies.HTTPClient, dependencies.Planner,
			dependencies.Authorization, dependencies.References, dependencies.Now, circleBreaker)
		if err != nil {
			return Plane{}, err
		}
		adapter = built
	}

	descriptor := providers.Descriptor{
		ID:      providers.ProviderCircleUserControlled,
		Version: 1,
		// Contract execution is provider capability metadata only. The actual
		// planner remains typed and allowlisted; it does not permit arbitrary
		// contract targets or calldata. Bridge and ANS are intentionally absent.
		Features: []capabilities.ProviderFeature{
			capabilities.FeatureUserControlledWallet,
			capabilities.FeatureContractExecution,
			capabilities.FeatureSwapExecution,
		},
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

	plane := Plane{
		Registry:      registry,
		Adapter:       adapter,
		CircleBreaker: circleBreaker,
		ArcBreaker:    arcBreaker,
	}

	circleHealth, err := circle.NewHealthChecker(config.Circle, dependencies.HTTPClient, circleBreaker, dependencies.Now)
	if err != nil {
		return Plane{}, err
	}
	arcHealth, err := arc.NewHealthChecker(config.Arc, dependencies.HTTPClient, arcBreaker, dependencies.Now)
	if err != nil {
		return Plane{}, err
	}
	plane.HealthCheckers = []providers.HealthChecker{circleHealth, arcHealth}

	if !config.Arc.Configured() || adapter == nil {
		// Without a chain reader or a reconciliation source there is no way to
		// obtain on-chain evidence, and a provider status alone may never
		// verify an execution.
		return plane, nil
	}
	client, err := arc.NewClientWithBreaker(config.Arc, dependencies.HTTPClient, arcBreaker)
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
	if dependencies.Intents != nil && dependencies.Payroll != nil && dependencies.Swap != nil && dependencies.PayrollV != nil && dependencies.SwapV != nil {
		composed, err := NewComposedVerifier(verifier, dependencies.Intents, dependencies.Payroll, dependencies.Swap, dependencies.PayrollV, dependencies.SwapV)
		if err != nil {
			return Plane{}, err
		}
		plane.DomainVerifier = composed
	}
	return plane, nil
}

// Health runs configured provider health checks under a bounded context.
// Process liveness endpoints must not require this result to succeed.
func (p Plane) Health(ctx context.Context) providers.HealthReport {
	return providers.AggregateHealth(ctx, time.Now, p.HealthCheckers...)
}

// ProviderFeatures reports the features actually available on a chain and
// network for capabilities.AvailabilityRequest.ProviderFeatures.
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
