// Package circuit implements a provider-neutral circuit breaker for outbound
// Circle and Arc calls. It never retries financial submissions and never
// classifies validation or user-authorization failures as infrastructure faults.
package circuit

import (
	"fmt"
	"sync"
	"time"
)

// State is the breaker state machine.
type State string

const (
	StateClosed   State = "CLOSED"
	StateOpen     State = "OPEN"
	StateHalfOpen State = "HALF_OPEN"
)

func (s State) Valid() bool {
	switch s {
	case StateClosed, StateOpen, StateHalfOpen:
		return true
	default:
		return false
	}
}

// Config bounds breaker behaviour. Zero values are rejected so operators must
// choose explicit thresholds rather than relying on hidden defaults at call sites.
type Config struct {
	// FailureThreshold is the consecutive infrastructure failures required to
	// open the breaker from CLOSED (or from a failed half-open probe).
	FailureThreshold int
	// OpenDuration is how long the breaker stays OPEN before admitting a
	// half-open probe.
	OpenDuration time.Duration
	// HalfOpenMaxProbes is the number of concurrent/sequential probes allowed
	// while HALF_OPEN. Successful recovery closes; any failure reopens.
	HalfOpenMaxProbes int
}

func (c Config) Validate() error {
	if c.FailureThreshold < 1 {
		return fmt.Errorf("circuit breaker failure threshold must be at least 1")
	}
	if c.OpenDuration <= 0 {
		return fmt.Errorf("circuit breaker open duration must be positive")
	}
	if c.HalfOpenMaxProbes < 1 {
		return fmt.Errorf("circuit breaker half-open probe limit must be at least 1")
	}
	return nil
}

// DefaultConfig is a conservative production-oriented starting point.
func DefaultConfig() Config {
	return Config{FailureThreshold: 5, OpenDuration: 30 * time.Second, HalfOpenMaxProbes: 1}
}

// ErrOpen is returned when the breaker refuses an outbound call.
var ErrOpen = fmt.Errorf("provider circuit breaker is open")

// Snapshot is safe breaker metadata for health reporting. It never carries
// credentials or request payloads.
type Snapshot struct {
	State            State
	ConsecutiveFails int
	OpenedAt         time.Time
	LastFailureAt    time.Time
	LastSuccessAt    time.Time
}

// Breaker is a concurrency-safe circuit breaker.
//
// Only infrastructure/provider-service failures should be recorded via
// RecordFailure. Caller validation errors and missing user authorization must
// not open the breaker.
type Breaker struct {
	mu               sync.Mutex
	config           Config
	now              func() time.Time
	state            State
	consecutiveFails int
	openedAt         time.Time
	lastFailureAt    time.Time
	lastSuccessAt    time.Time
	halfOpenProbes   int
}

// New constructs a breaker in CLOSED state.
func New(config Config, now func() time.Time) (*Breaker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	return &Breaker{config: config, now: now, state: StateClosed}, nil
}

// Allow reports whether an outbound call may proceed. When the breaker is OPEN
// and the cooldown has elapsed it transitions to HALF_OPEN for a probe.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now().UTC()
	switch b.state {
	case StateClosed:
		return nil
	case StateOpen:
		if now.Before(b.openedAt.Add(b.config.OpenDuration)) {
			return ErrOpen
		}
		b.state = StateHalfOpen
		b.halfOpenProbes = 1
		return nil
	case StateHalfOpen:
		if b.halfOpenProbes >= b.config.HalfOpenMaxProbes {
			return ErrOpen
		}
		b.halfOpenProbes++
		return nil
	default:
		return ErrOpen
	}
}

// RecordSuccess marks a successful provider infrastructure call and closes the
// breaker.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consecutiveFails = 0
	b.halfOpenProbes = 0
	b.state = StateClosed
	b.openedAt = time.Time{}
	b.lastSuccessAt = b.now().UTC()
}

// RecordFailure records a provider infrastructure failure. Validation and
// user-authorization failures must not call this method.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now().UTC()
	b.lastFailureAt = now
	b.consecutiveFails++
	switch b.state {
	case StateHalfOpen:
		b.openLocked(now)
	case StateClosed:
		if b.consecutiveFails >= b.config.FailureThreshold {
			b.openLocked(now)
		}
	case StateOpen:
		// Stay open; cooldown already governs probes.
	}
}

func (b *Breaker) openLocked(now time.Time) {
	b.state = StateOpen
	b.openedAt = now
	b.halfOpenProbes = 0
}

// Snapshot returns a defensive copy of breaker metadata.
func (b *Breaker) Snapshot() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Snapshot{
		State:            b.state,
		ConsecutiveFails: b.consecutiveFails,
		OpenedAt:         b.openedAt,
		LastFailureAt:    b.lastFailureAt,
		LastSuccessAt:    b.lastSuccessAt,
	}
}

// State returns the current breaker state.
func (b *Breaker) State() State {
	return b.Snapshot().State
}
