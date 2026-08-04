package circuit_test

import (
	"sync"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/providers/circuit"
)

func TestBreakerOpensAfterThresholdAndRecovers(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	breaker, err := circuit.New(circuit.Config{FailureThreshold: 3, OpenDuration: time.Minute, HalfOpenMaxProbes: 1}, clock)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := breaker.Allow(); err != nil {
			t.Fatalf("allow before open: %v", err)
		}
		breaker.RecordFailure()
	}
	if breaker.State() != circuit.StateClosed {
		t.Fatalf("state = %s, want CLOSED", breaker.State())
	}
	if err := breaker.Allow(); err != nil {
		t.Fatal(err)
	}
	breaker.RecordFailure()
	if breaker.State() != circuit.StateOpen {
		t.Fatalf("state = %s, want OPEN", breaker.State())
	}
	if err := breaker.Allow(); err != circuit.ErrOpen {
		t.Fatalf("open breaker allow = %v", err)
	}

	now = now.Add(time.Minute + time.Second)
	if err := breaker.Allow(); err != nil {
		t.Fatalf("half-open probe allow: %v", err)
	}
	if breaker.State() != circuit.StateHalfOpen {
		t.Fatalf("state = %s, want HALF_OPEN", breaker.State())
	}
	// Second probe while half-open exceeds limit.
	if err := breaker.Allow(); err != circuit.ErrOpen {
		t.Fatalf("second half-open probe = %v", err)
	}
	breaker.RecordSuccess()
	if breaker.State() != circuit.StateClosed {
		t.Fatalf("after success state = %s", breaker.State())
	}
	if err := breaker.Allow(); err != nil {
		t.Fatal(err)
	}
}

func TestBreakerHalfOpenFailureReopens(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	breaker, err := circuit.New(circuit.Config{FailureThreshold: 1, OpenDuration: time.Second, HalfOpenMaxProbes: 1}, clock)
	if err != nil {
		t.Fatal(err)
	}
	_ = breaker.Allow()
	breaker.RecordFailure()
	now = now.Add(2 * time.Second)
	_ = breaker.Allow()
	breaker.RecordFailure()
	if breaker.State() != circuit.StateOpen {
		t.Fatalf("state = %s, want OPEN", breaker.State())
	}
}

func TestBreakerConcurrentAllowAndRecord(t *testing.T) {
	breaker, err := circuit.New(circuit.DefaultConfig(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = breaker.Allow()
			if i%2 == 0 {
				breaker.RecordFailure()
			} else {
				breaker.RecordSuccess()
			}
			_ = breaker.Snapshot()
		}(i)
	}
	wg.Wait()
	if !breaker.State().Valid() {
		t.Fatalf("invalid state %q", breaker.State())
	}
}

func TestBreakerConfigValidation(t *testing.T) {
	if _, err := circuit.New(circuit.Config{}, time.Now); err == nil {
		t.Fatal("empty config must fail")
	}
}
