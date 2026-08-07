package autonomy

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"
)

type EmergencyStop struct {
	Active    bool
	Scope     string
	ChangedAt time.Time
	Actor     string
	Reason    string
}

func (e EmergencyStop) Validate() error {
	if e.Scope != "TENANT" {
		return fmt.Errorf("emergency stop scope must be TENANT")
	}
	if e.Actor == "" || len(e.Actor) > 256 || e.ChangedAt.IsZero() || e.Reason == "" || len(e.Reason) > 256 {
		return fmt.Errorf("emergency stop state is invalid")
	}
	return nil
}

type Decision struct {
	Eligible                          bool
	RequiresStepUp                    bool
	Reason                            ReasonCode
	RemainingBaseUnits                string
	ScheduleID, OccurrenceID, GrantID string
}
type DispatchGuard struct {
	OccurrenceID, ScheduleID, GrantID, PrincipalUserID, AgentID, DelegationID, LeaseOwner string
	ScheduleVersion, GrantVersion, DelegationVersion, Fence                               uint64
}

// Store is deliberately narrower than the existing execution repositories. A
// PostgreSQL implementation can map these operations to SELECT FOR UPDATE and
// fencing-token updates; implementations must make Reserve atomic.
type Store interface {
	SaveSchedule(context.Context, Schedule) error
	GetSchedule(context.Context, string, uint64) (Schedule, error)
	SaveOccurrence(context.Context, Occurrence) error
	GetOccurrence(context.Context, string) (Occurrence, error)
	ClaimDue(context.Context, time.Time, string, time.Duration) (Occurrence, bool, error)
	Reserve(context.Context, string, string, uint64, string, time.Time) (bool, error)
	ReleaseReservation(context.Context, string) error
	Emergency(context.Context) (EmergencyStop, error)
	SetEmergency(context.Context, EmergencyStop) error
	CheckDispatchFence(context.Context, string, string, uint64, time.Time) (Occurrence, bool, error)
}

type MemoryStore struct {
	mu           sync.Mutex
	schedules    map[string]Schedule
	occurrences  map[string]Occurrence
	reservations map[string]reservation
	stop         EmergencyStop
}
type reservation struct {
	GrantID, Amount string
	GrantVersion    uint64
	ReservedAt      time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{schedules: map[string]Schedule{}, occurrences: map[string]Occurrence{}, reservations: map[string]reservation{}}
}
func (m *MemoryStore) SaveSchedule(_ context.Context, s Schedule) error {
	if err := s.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s/%d", s.ID, s.Version)
	if existing, ok := m.schedules[key]; ok {
		if existing.ComputeDigest() != s.ComputeDigest() || existing.Status != s.Status {
			return fmt.Errorf("schedule immutable conflict")
		}
		return nil
	}
	m.schedules[key] = s
	return nil
}
func (m *MemoryStore) GetSchedule(_ context.Context, id string, v uint64) (Schedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.schedules[fmt.Sprintf("%s/%d", id, v)]
	if !ok {
		return Schedule{}, fmt.Errorf("schedule not found")
	}
	return s, nil
}
func (m *MemoryStore) SaveOccurrence(_ context.Context, o Occurrence) error {
	if err := o.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.occurrences[o.Key]; ok && existing.ID != o.ID {
		return fmt.Errorf("occurrence key collision")
	}
	if existing, ok := m.occurrences[o.Key]; ok {
		if existing.ScheduleDigest != o.ScheduleDigest || existing.GrantID != o.GrantID || existing.GrantVersion != o.GrantVersion {
			return fmt.Errorf("occurrence immutable conflict")
		}
		return nil
	}
	m.occurrences[o.Key] = o
	return nil
}
func (m *MemoryStore) GetOccurrence(_ context.Context, id string) (Occurrence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, o := range m.occurrences {
		if o.ID == id {
			return o, nil
		}
	}
	return Occurrence{}, fmt.Errorf("occurrence not found")
}
func (m *MemoryStore) ClaimDue(_ context.Context, at time.Time, worker string, lease time.Duration) (Occurrence, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, o := range m.occurrences {
		if (o.Status != OccurrenceDue && o.Status != OccurrenceClaimed) || o.ScheduledAt.After(at) {
			continue
		}
		if o.LeaseOwner != "" && o.LeaseUntil.After(at) {
			continue
		}
		o.Status = OccurrenceClaimed
		o.LeaseOwner = worker
		o.LeaseUntil = at.Add(lease)
		o.Fence++
		m.occurrences[key] = o
		return o, true, nil
	}
	return Occurrence{}, false, nil
}
func (m *MemoryStore) Reserve(_ context.Context, occ, grant string, grantVersion uint64, amount string, at time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.reservations[occ]; ok {
		return true, nil
	}
	z, ok := new(big.Int).SetString(amount, 10)
	if !ok || z.Sign() <= 0 {
		return false, fmt.Errorf("invalid reservation amount")
	}
	m.reservations[occ] = reservation{GrantID: grant, GrantVersion: grantVersion, Amount: amount, ReservedAt: at}
	return true, nil
}
func (m *MemoryStore) ReleaseReservation(_ context.Context, occ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.reservations, occ)
	return nil
}
func (m *MemoryStore) Emergency(_ context.Context) (EmergencyStop, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stop, nil
}
func (m *MemoryStore) SetEmergency(_ context.Context, s EmergencyStop) error {
	if err := s.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stop = s
	return nil
}
func (m *MemoryStore) CheckDispatchFence(_ context.Context, id, worker string, fence uint64, at time.Time) (Occurrence, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, o := range m.occurrences {
		if o.ID != id {
			continue
		}
		if o.Status != OccurrenceClaimed || o.LeaseOwner != worker || o.Fence != fence || !o.LeaseUntil.After(at) {
			return o, false, nil
		}
		return o, true, nil
	}
	return Occurrence{}, false, fmt.Errorf("occurrence not found")
}

type Runtime struct {
	Store    Store
	Now      func() time.Time
	Enabled  bool
	WorkerID string
}

func (r Runtime) validate() error {
	if r.Store == nil || r.Now == nil || r.WorkerID == "" {
		return fmt.Errorf("autonomy runtime dependencies are required")
	}
	return nil
}
func (r Runtime) Simulate(ctx context.Context, s Schedule, o Occurrence, g Grant, p Principal, delegation Delegation, amount, recipient, token, chain string) (Decision, error) {
	if err := s.Validate(); err != nil {
		return Decision{}, err
	}
	if err := g.Validate(); err != nil {
		return Decision{}, err
	}
	if err := p.Validate(); err != nil {
		return Decision{}, err
	}
	decision := Decision{Eligible: false, Reason: ReasonEligible, ScheduleID: s.ID, OccurrenceID: o.ID, GrantID: g.ID}
	if delegation.PrincipalUserID != p.UserID || delegation.AgentID != p.AgentID || s.Principal.UserID != p.UserID || s.Principal.AgentID != p.AgentID || s.GrantID != g.ID || s.GrantVersion != g.Version || s.DelegationID != delegation.ID || s.DelegationVersion != delegation.Version || o.ScheduleID != s.ID || o.ScheduleVersion != s.Version || o.GrantID != g.ID || o.GrantVersion != g.Version {
		decision.Reason = ReasonGrantDenied
		return decision, nil
	}
	if dErr := delegation.Validate(r.Now(), s.Spec.Intent); dErr != nil {
		decision.Reason = ReasonDelegationDenied
		return decision, nil
	}
	if !r.Enabled {
		decision.Reason = ReasonRuntimeDisabled
		return decision, nil
	}
	if s.Status != ScheduleActive {
		decision.Reason = ReasonSchedulePaused
		return decision, nil
	}
	if !g.Active(r.Now()) || g.PrincipalUserID != p.UserID || g.WalletBindingID != s.WalletBindingID || g.ScheduleID != "" && g.ScheduleID != s.ID {
		decision.Reason = ReasonGrantDenied
		return decision, nil
	}
	if !allowed(g.AllowedRecipients, recipient) || !allowed(g.AllowedTokens, token) || !allowed(g.AllowedChains, chain) {
		decision.Reason = ReasonGrantDenied
		return decision, nil
	}
	z, ok := new(big.Int).SetString(amount, 10)
	if !ok || z.Sign() <= 0 {
		return Decision{}, fmt.Errorf("amount must be positive base units")
	}
	if g.PerActionBaseUnits != "" {
		cap, _ := new(big.Int).SetString(g.PerActionBaseUnits, 10)
		if z.Cmp(cap) > 0 {
			decision.Reason = ReasonGrantDenied
			return decision, nil
		}
	}
	if g.StepUpAboveBaseUnits != "" {
		threshold, _ := new(big.Int).SetString(g.StepUpAboveBaseUnits, 10)
		if z.Cmp(threshold) > 0 {
			decision.RequiresStepUp = true
			decision.Reason = ReasonStepUp
			return decision, nil
		}
	}
	decision.Eligible = true
	decision.RemainingBaseUnits = g.AggregateCapBaseUnits
	return decision, nil
}

// BeforeDispatch is the last fail-closed gate. It is intentionally separate
// from planning and provider execution; callers must pass the immutable
// occurrence identity and never reconstruct it from mutable schedule data.
func (r Runtime) BeforeDispatch(ctx context.Context, g DispatchGuard, s Schedule, grant Grant, p Principal, d Delegation) (ReasonCode, error) {
	if err := r.validate(); err != nil {
		return "", err
	}
	stop, err := r.Store.Emergency(ctx)
	if err != nil {
		return "", err
	}
	if stop.Active || stop.Scope != "" || stop.Actor != "" || stop.Reason != "" || !stop.ChangedAt.IsZero() {
		if err := stop.Validate(); err != nil {
			return ReasonEmergencyStop, err
		}
	}
	if stop.Active {
		return ReasonEmergencyStop, nil
	}
	if !r.Enabled {
		return ReasonRuntimeDisabled, nil
	}
	if s.Status != ScheduleActive {
		return ReasonSchedulePaused, nil
	}
	if s.GrantID != grant.ID || s.GrantVersion != grant.Version || g.GrantID != grant.ID || g.GrantVersion != grant.Version || g.ScheduleID != s.ID || g.ScheduleVersion != s.Version {
		return ReasonGrantDenied, nil
	}
	if !grant.Active(r.Now()) || grant.PrincipalUserID != p.UserID || grant.WalletBindingID != s.WalletBindingID || (grant.ScheduleID != "" && grant.ScheduleID != s.ID) {
		return ReasonGrantDenied, nil
	}
	if d.PrincipalUserID != p.UserID || d.AgentID != p.AgentID || s.Principal.UserID != p.UserID || s.Principal.AgentID != p.AgentID || g.PrincipalUserID != p.UserID || g.AgentID != p.AgentID || g.DelegationID != d.ID || g.DelegationVersion != d.Version || s.DelegationID != d.ID || s.DelegationVersion != d.Version {
		return ReasonDelegationDenied, nil
	}
	if err := d.Validate(r.Now(), s.Spec.Intent); err != nil {
		return ReasonDelegationDenied, nil
	}
	if g.OccurrenceID == "" || g.LeaseOwner == "" {
		return ReasonGrantDenied, nil
	}
	occurrence, valid, err := r.Store.CheckDispatchFence(ctx, g.OccurrenceID, g.LeaseOwner, g.Fence, r.Now())
	if err != nil {
		return "", err
	}
	if !valid {
		return ReasonGrantDenied, nil
	}
	if occurrence.GrantID != grant.ID || occurrence.GrantVersion != grant.Version || occurrence.ScheduleID != s.ID || occurrence.ScheduleVersion != s.Version {
		return ReasonGrantDenied, nil
	}
	return ReasonEligible, nil
}
