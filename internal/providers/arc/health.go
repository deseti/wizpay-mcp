package arc

import (
	"context"
	"net/http"
	"time"

	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/providers/circuit"
)

// HealthChecker probes Arc RPC availability and chain identity using only
// allowlisted read methods (eth_chainId, eth_blockNumber).
type HealthChecker struct {
	config  Config
	client  *Client
	breaker *circuit.Breaker
	now     func() time.Time
}

// NewHealthChecker builds an Arc health checker. When Arc is not configured it
// reports NOT_CONFIGURED without network I/O.
func NewHealthChecker(config Config, httpClient *http.Client, breaker *circuit.Breaker, now func() time.Time) (*HealthChecker, error) {
	if now == nil {
		now = time.Now
	}
	checker := &HealthChecker{config: config, breaker: breaker, now: now}
	if !config.Configured() {
		return checker, nil
	}
	client, err := NewClientWithBreaker(config, httpClient, breaker)
	if err != nil {
		return nil, err
	}
	checker.client = client
	return checker, nil
}

func (h *HealthChecker) Name() string { return "arc" }

// Check validates RPC reachability and chain ID 5042002, and reads block height.
func (h *HealthChecker) Check(ctx context.Context) providers.ComponentHealth {
	observed := h.now().UTC()
	result := providers.ComponentHealth{Name: h.Name(), CheckedAt: observed}
	if h.breaker != nil {
		result.BreakerState = string(h.breaker.State())
	}
	if h == nil || !h.config.Configured() {
		result.Status = providers.HealthNotConfigured
		result.Detail = "Arc chain adapter is not configured"
		return result
	}
	if h.client == nil {
		result.Status = providers.HealthUnavailable
		result.Detail = "Arc health client is unavailable"
		return result
	}

	chainID, blockNumber, err := h.client.HealthCheck(ctx)
	if err != nil {
		result.Status = providers.HealthUnavailable
		result.Detail = "Arc RPC is unavailable or chain identity failed"
		if h.breaker != nil {
			result.BreakerState = string(h.breaker.State())
		}
		return result
	}
	if chainID != ChainIDTestnet {
		result.Status = providers.HealthUnavailable
		result.Detail = "Arc chain ID mismatch"
		return result
	}
	if blockNumber == 0 {
		result.Status = providers.HealthDegraded
		result.Detail = "Arc chain ID verified but block height is zero"
		return result
	}
	result.Status = providers.HealthHealthy
	result.Detail = "Arc RPC chain identity and block height verified"
	if h.breaker != nil {
		result.BreakerState = string(h.breaker.State())
	}
	return result
}
