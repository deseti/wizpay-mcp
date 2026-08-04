package providers

import (
	"context"
	"testing"
	"time"
)

type stubChecker struct {
	name   string
	health ComponentHealth
	panic  bool
}

func (s stubChecker) Name() string { return s.name }
func (s stubChecker) Check(context.Context) ComponentHealth {
	if s.panic {
		panic("boom")
	}
	return s.health
}

func TestAggregateHealthWorstStatusAndPanicRecovery(t *testing.T) {
	now := providerTestNow
	report := AggregateHealth(context.Background(), func() time.Time { return now },
		stubChecker{name: "a", health: ComponentHealth{Name: "a", Status: HealthHealthy, CheckedAt: now}},
		stubChecker{name: "b", health: ComponentHealth{Name: "b", Status: HealthDegraded, CheckedAt: now}},
		stubChecker{name: "c", panic: true},
	)
	if report.Overall() != HealthUnavailable {
		t.Fatalf("overall = %s", report.Overall())
	}
	if len(report.Components) != 3 {
		t.Fatalf("components = %d", len(report.Components))
	}
	if report.Components[2].Status != HealthUnavailable {
		t.Fatalf("panic component = %#v", report.Components[2])
	}
}

func TestHealthReportEmptyIsNotConfigured(t *testing.T) {
	if (HealthReport{}).Overall() != HealthNotConfigured {
		t.Fatal("empty report should be NOT_CONFIGURED")
	}
}
