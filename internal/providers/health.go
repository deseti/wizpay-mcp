package providers

import (
	"context"
	"fmt"
	"time"
)

// HealthStatus is the provider-plane health classification.
type HealthStatus string

const (
	HealthHealthy       HealthStatus = "HEALTHY"
	HealthDegraded      HealthStatus = "DEGRADED"
	HealthUnavailable   HealthStatus = "UNAVAILABLE"
	HealthNotConfigured HealthStatus = "NOT_CONFIGURED"
)

func (s HealthStatus) Valid() bool {
	switch s {
	case HealthHealthy, HealthDegraded, HealthUnavailable, HealthNotConfigured:
		return true
	default:
		return false
	}
}

// ComponentHealth is a safe, non-secret health observation for one provider or
// chain dependency.
type ComponentHealth struct {
	Name   string
	Status HealthStatus
	// Detail is operator-safe text only. It must never contain API keys, user
	// tokens, request bodies, or response bodies.
	Detail    string
	CheckedAt time.Time
	// BreakerState is optional circuit-breaker metadata when applicable.
	BreakerState string
}

func (h ComponentHealth) Validate() error {
	if h.Name == "" {
		return fmt.Errorf("health component name is required")
	}
	if !h.Status.Valid() {
		return fmt.Errorf("health status is invalid")
	}
	if h.CheckedAt.IsZero() {
		return fmt.Errorf("health check time is required")
	}
	return nil
}

// HealthReport aggregates provider-plane health without secrets.
type HealthReport struct {
	CheckedAt  time.Time
	Components []ComponentHealth
}

// Overall returns the worst status among components. Empty reports are
// NOT_CONFIGURED.
func (r HealthReport) Overall() HealthStatus {
	if len(r.Components) == 0 {
		return HealthNotConfigured
	}
	worst := HealthHealthy
	rank := map[HealthStatus]int{
		HealthHealthy:       0,
		HealthNotConfigured: 1,
		HealthDegraded:      2,
		HealthUnavailable:   3,
	}
	for _, component := range r.Components {
		if rank[component.Status] > rank[worst] {
			worst = component.Status
		}
	}
	return worst
}

// HealthChecker is a bounded provider/chain health probe.
type HealthChecker interface {
	// Name identifies the component in health reports.
	Name() string
	// Check must respect context cancellation/timeouts, never panic, and never
	// return secrets in Detail.
	Check(ctx context.Context) ComponentHealth
}

// AggregateHealth runs every checker with the parent context (callers should
// apply an overall deadline). Individual checker panics are recovered as
// UNAVAILABLE.
func AggregateHealth(ctx context.Context, now func() time.Time, checkers ...HealthChecker) HealthReport {
	if now == nil {
		now = time.Now
	}
	report := HealthReport{CheckedAt: now().UTC()}
	for _, checker := range checkers {
		if checker == nil {
			continue
		}
		report.Components = append(report.Components, safeCheck(ctx, checker, now))
	}
	return report
}

func safeCheck(ctx context.Context, checker HealthChecker, now func() time.Time) (result ComponentHealth) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = ComponentHealth{
				Name:      checker.Name(),
				Status:    HealthUnavailable,
				Detail:    "health check panicked",
				CheckedAt: now().UTC(),
			}
		}
	}()
	result = checker.Check(ctx)
	if result.Name == "" {
		result.Name = checker.Name()
	}
	if result.CheckedAt.IsZero() {
		result.CheckedAt = now().UTC()
	}
	if !result.Status.Valid() {
		result.Status = HealthUnavailable
		result.Detail = "health check returned an invalid status"
	}
	return result
}
