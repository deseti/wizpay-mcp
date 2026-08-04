package circle

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/providers/circuit"
)

// HealthChecker probes Circle reachability without performing financial actions.
// It never creates wallets, transfers tokens, or completes challenges.
type HealthChecker struct {
	config  Config
	client  *client
	breaker *circuit.Breaker
	now     func() time.Time
}

// NewHealthChecker builds a Circle health checker. When the provider is not
// configured it reports NOT_CONFIGURED without network I/O.
func NewHealthChecker(config Config, httpClient *http.Client, breaker *circuit.Breaker, now func() time.Time) (*HealthChecker, error) {
	if now == nil {
		now = time.Now
	}
	checker := &HealthChecker{config: config, breaker: breaker, now: now}
	if !config.Configured() {
		return checker, nil
	}
	transport, err := newClientWithBreaker(config, httpClient, breaker)
	if err != nil {
		return nil, err
	}
	checker.client = transport
	return checker, nil
}

func (h *HealthChecker) Name() string { return "circle" }

// Check performs a bounded non-financial reachability probe.
func (h *HealthChecker) Check(ctx context.Context) providers.ComponentHealth {
	observed := h.now().UTC()
	result := providers.ComponentHealth{Name: h.Name(), CheckedAt: observed}
	if h.breaker != nil {
		result.BreakerState = string(h.breaker.State())
	}
	if h == nil || !h.config.Configured() {
		result.Status = providers.HealthNotConfigured
		result.Detail = "Circle provider is not configured"
		return result
	}
	if h.client == nil {
		result.Status = providers.HealthUnavailable
		result.Detail = "Circle health client is unavailable"
		return result
	}

	statusCode, err := h.client.healthProbe(ctx)
	if err != nil {
		if errors.Is(err, circuit.ErrOpen) {
			result.Status = providers.HealthDegraded
			result.Detail = "Circle circuit breaker is open"
			return result
		}
		result.Status = providers.HealthUnavailable
		result.Detail = "Circle endpoint is unreachable"
		return result
	}
	switch {
	case statusCode >= 200 && statusCode < 300:
		result.Status = providers.HealthHealthy
		result.Detail = "Circle endpoint answered successfully"
	case statusCode == 401 || statusCode == 403:
		// Host is up; credentials rejected. Degraded, not a financial error.
		result.Status = providers.HealthDegraded
		result.Detail = "Circle endpoint rejected credentials"
	case statusCode == 404:
		// Reachable API host; path may differ by environment.
		result.Status = providers.HealthHealthy
		result.Detail = "Circle endpoint is reachable"
	default:
		result.Status = providers.HealthDegraded
		result.Detail = "Circle endpoint returned an unexpected status"
	}
	if h.breaker != nil {
		result.BreakerState = string(h.breaker.State())
	}
	return result
}
