package autonomy

import (
	"fmt"
	"math/big"
	"sync"
	"time"
)

// Ledger provides transactional reservation semantics. Production storage
// implements the same operation with a unique (grant, occurrence) key and a
// row lock; this reference implementation is used by deterministic tests.
type Ledger struct {
	mu           sync.Mutex
	reservations map[string]LedgerReservation
}
type LedgerReservation struct {
	GrantID, OccurrenceID, Amount string
	GrantVersion                  uint64
	At                            time.Time
	State                         string
}

func NewLedger() *Ledger { return &Ledger{reservations: map[string]LedgerReservation{}} }
func (l *Ledger) Reserve(g Grant, occurrence, amount string, at time.Time) (bool, error) {
	if err := g.Validate(); err != nil {
		return false, err
	}
	z, ok := new(big.Int).SetString(amount, 10)
	if !ok || z.Sign() <= 0 {
		return false, fmt.Errorf("reservation amount must be positive")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if old, ok := l.reservations[occurrence]; ok {
		return old.GrantID == g.ID && old.GrantVersion == g.Version, nil
	}
	var total big.Int
	var window big.Int
	for _, r := range l.reservations {
		if r.GrantID != g.ID || r.GrantVersion != g.Version || (r.State != "RESERVED" && r.State != "COMMITTED") {
			continue
		}
		v, _ := new(big.Int).SetString(r.Amount, 10)
		total.Add(&total, v)
		if g.RollingWindow > 0 && at.Sub(r.At) < g.RollingWindow {
			window.Add(&window, v)
		}
	}
	if g.AggregateCapBaseUnits != "" {
		cap, _ := new(big.Int).SetString(g.AggregateCapBaseUnits, 10)
		var candidate big.Int
		candidate.Add(&total, z)
		if candidate.Cmp(cap) > 0 {
			return false, nil
		}
	}
	if g.RollingWindowCapBaseUnits != "" {
		cap, _ := new(big.Int).SetString(g.RollingWindowCapBaseUnits, 10)
		var candidate big.Int
		candidate.Add(&window, z)
		if candidate.Cmp(cap) > 0 {
			return false, nil
		}
	}
	l.reservations[occurrence] = LedgerReservation{GrantID: g.ID, GrantVersion: g.Version, OccurrenceID: occurrence, Amount: amount, At: at, State: "RESERVED"}
	return true, nil
}
func (l *Ledger) Commit(occurrence string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.reservations[occurrence]
	if !ok {
		return fmt.Errorf("reservation not found")
	}
	r.State = "COMMITTED"
	l.reservations[occurrence] = r
	return nil
}
func (l *Ledger) Release(occurrence string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if r, ok := l.reservations[occurrence]; ok && r.State == "RESERVED" {
		delete(l.reservations, occurrence)
	}
	return nil
}
func (l *Ledger) Get(occurrence string) (LedgerReservation, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.reservations[occurrence]
	return r, ok
}
