package autonomy

import (
	"context"
	"sync"
	"testing"
	"time"
)

func testSchedule(t *testing.T) Schedule {
	t.Helper()
	loc := time.FixedZone("UTC", 0)
	start := time.Date(2026, 1, 1, 9, 0, 0, 0, loc)
	spec := ScheduleSpec{Recurrence: Recurrence{Frequency: Daily, Start: start, Location: "UTC"}, Missed: MissedRunLatest, Concurrency: ForbidOverlap, Intent: IntentPayroll, TemplateDigest: "sha256:typed", MaxRecipients: 10}
	s := Schedule{ID: "sch_1", Version: 1, Principal: Principal{TenantID: "ten_1", UserID: "usr_1", AgentID: "agent_1"}, WalletBindingID: "wal_1", WalletBindingVersion: 1, GrantID: "grant_1", GrantVersion: 1, DelegationID: "del_1", DelegationVersion: 1, CreatedAt: start, UpdatedAt: start, Status: ScheduleActive, Spec: spec}
	s.Digest = s.ComputeDigest()
	return s
}
func testGrant() Grant {
	return Grant{ID: "grant_1", Version: 1, PrincipalUserID: "usr_1", WalletBindingID: "wal_1", Intent: IntentPayroll, ExpiresAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), AggregateCapBaseUnits: "100", RollingWindowCapBaseUnits: "60", RollingWindow: time.Hour, PerActionBaseUnits: "50", StepUpAboveBaseUnits: "90"}
}
func testDelegation() Delegation {
	return Delegation{ID: "del_1", Version: 1, PrincipalUserID: "usr_1", AgentID: "agent_1", Capabilities: []IntentType{IntentPayroll}, ExpiresAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), NonTransitive: true}
}
func TestDeterministicScheduleAndOccurrence(t *testing.T) {
	s := testSchedule(t)
	if s.ComputeDigest() != s.ComputeDigest() {
		t.Fatal("digest not deterministic")
	}
	at := s.Spec.Recurrence.Start
	one, two := NewOccurrence(s, at), NewOccurrence(s, at)
	if one.ID != two.ID || one.Key != two.Key {
		t.Fatal("occurrence identity not deterministic")
	}
	changed := s
	changed.GrantVersion = 2
	if changed.ComputeDigest() == s.ComputeDigest() {
		t.Fatal("grant version must change schedule digest")
	}
}

func TestDelegationAndScheduleLifecycleValidation(t *testing.T) {
	s := testSchedule(t)
	if !ValidScheduleTransition(ScheduleActive, SchedulePaused) || !ValidScheduleTransition(SchedulePaused, ScheduleActive) || ValidScheduleTransition(ScheduleRevoked, ScheduleActive) {
		t.Fatal("invalid schedule transition matrix")
	}
	bad := testDelegation()
	bad.NonTransitive = false
	if bad.ValidateStructure() == nil {
		t.Fatal("transitive delegation accepted")
	}
	bad.Capabilities = []IntentType{"BRIDGE"}
	if bad.ValidateStructure() == nil {
		t.Fatal("unsupported delegation capability accepted")
	}
	_ = s
}
func TestMissedRunPolicies(t *testing.T) {
	s := testSchedule(t)
	from := s.Spec.Recurrence.Start
	now := from.Add(5 * 24 * time.Hour)
	skip := s.Spec
	skip.Missed = MissedSkip
	if got, _ := MissedInstants(skip, from, now, 10); len(got) != 0 {
		t.Fatal("skip must not catch up")
	}
	got, err := MissedInstants(s.Spec, from, now, 10)
	if err != nil || len(got) != 1 || !got[0].Equal(from.Add(5*24*time.Hour)) {
		t.Fatalf("run latest = %v, %v", got, err)
	}
}
func TestConcurrentClaimsHaveOneWinner(t *testing.T) {
	store := NewMemoryStore()
	s := testSchedule(t)
	if err := store.SaveSchedule(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	o := NewOccurrence(s, s.Spec.Recurrence.Start)
	if err := store.SaveOccurrence(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var wins int
	var mu sync.Mutex
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, ok, err := store.ClaimDue(context.Background(), s.Spec.Recurrence.Start.Add(time.Hour), "w", time.Minute)
			if err != nil {
				t.Error(err)
			}
			if ok {
				mu.Lock()
				wins++
				_ = got
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("claim winners=%d", wins)
	}
}
func TestGrantCapsAndIdempotentReservation(t *testing.T) {
	g := testGrant()
	l := NewLedger()
	at := time.Now()
	ok, err := l.Reserve(g, "occ1", "50", at)
	if err != nil || !ok {
		t.Fatal(err)
	}
	ok, err = l.Reserve(g, "occ1", "50", at)
	if err != nil || !ok {
		t.Fatal("retry must be idempotent")
	}
	if err := l.Commit("occ1"); err != nil {
		t.Fatal(err)
	}
	ok, err = l.Reserve(g, "occ1", "50", at)
	if err != nil || !ok {
		t.Fatal("committed replay must be idempotent")
	}
	ok, err = l.Reserve(g, "occ_committed_2", "11", at)
	if err != nil || ok {
		t.Fatal("committed spend must continue consuming aggregate cap")
	}
	ok, err = l.Reserve(g, "occ2", "20", at)
	if err != nil || ok {
		t.Fatal("rolling cap must deny overspend")
	}
	g2 := g
	g2.ID = "grant_2"
	g2.AggregateCapBaseUnits = "20"
	g2.RollingWindowCapBaseUnits = ""
	g2.RollingWindow = 0
	ok, err = l.Reserve(g2, "occ_release", "10", at)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err := l.Release("occ_release"); err != nil {
		t.Fatal(err)
	}
	ok, err = l.Reserve(g2, "occ_after_release", "20", at)
	if err != nil || !ok {
		t.Fatal("released reservation should not consume budget")
	}
}
func TestSimulationAndEmergencyStop(t *testing.T) {
	s := testSchedule(t)
	o := NewOccurrence(s, s.Spec.Recurrence.Start)
	g := testGrant()
	d := testDelegation()
	r := Runtime{Store: NewMemoryStore(), Now: func() time.Time { return time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC) }, Enabled: true, WorkerID: "worker"}
	decision, err := r.Simulate(context.Background(), s, o, g, s.Principal, d, "10", "recipient", "token", "5042002")
	if err != nil || !decision.Eligible {
		t.Fatalf("simulation should be eligible: %+v %v", decision, err)
	}
	if err := r.Store.SetEmergency(context.Background(), EmergencyStop{Active: true, Scope: "TENANT", Actor: "usr_1", Reason: "incident", ChangedAt: r.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := r.Store.SaveOccurrence(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := r.Store.ClaimDue(context.Background(), r.Now(), "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim for dispatch: %v", err)
	}
	o = claimed
	reason, err := r.BeforeDispatch(context.Background(), DispatchGuard{OccurrenceID: o.ID, ScheduleID: s.ID, ScheduleVersion: s.Version, GrantID: g.ID, GrantVersion: g.Version, PrincipalUserID: "usr_1", AgentID: "agent_1", DelegationID: d.ID, DelegationVersion: d.Version, LeaseOwner: "worker", Fence: o.Fence}, s, g, s.Principal, d)
	if err != nil || reason != ReasonEmergencyStop {
		t.Fatalf("stop reason=%s err=%v", reason, err)
	}
}

func TestDelegationPrincipalAndAgentSubstitutionRejected(t *testing.T) {
	s := testSchedule(t)
	o := NewOccurrence(s, s.Spec.Recurrence.Start)
	g := testGrant()
	store := NewMemoryStore()
	r := Runtime{Store: store, Now: func() time.Time { return time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC) }, Enabled: true, WorkerID: "worker"}
	if err := store.SaveOccurrence(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := store.ClaimDue(context.Background(), r.Now(), "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %v", err)
	}
	o = claimed
	guard := DispatchGuard{OccurrenceID: o.ID, ScheduleID: s.ID, ScheduleVersion: s.Version, GrantID: g.ID, GrantVersion: g.Version, PrincipalUserID: s.Principal.UserID, AgentID: s.Principal.AgentID, DelegationID: "del_1", DelegationVersion: 1, LeaseOwner: "worker", Fence: o.Fence}
	for _, test := range []struct {
		name   string
		mutate func(*Delegation)
	}{
		{name: "principal user", mutate: func(d *Delegation) { d.PrincipalUserID = "other-user" }},
		{name: "agent", mutate: func(d *Delegation) { d.AgentID = "other-agent" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := testDelegation()
			test.mutate(&d)
			decision, err := r.Simulate(context.Background(), s, o, g, s.Principal, d, "10", "recipient", "token", "5042002")
			if err != nil || decision.Reason != ReasonGrantDenied {
				t.Fatalf("simulate substitution accepted: decision=%+v err=%v", decision, err)
			}
			reason, err := r.BeforeDispatch(context.Background(), guard, s, g, s.Principal, d)
			if err != nil || reason != ReasonDelegationDenied {
				t.Fatalf("dispatch substitution accepted: reason=%s err=%v", reason, err)
			}
		})
	}
}

func TestDispatchFenceRejectsReclaimedLease(t *testing.T) {
	s := testSchedule(t)
	o := NewOccurrence(s, s.Spec.Recurrence.Start)
	store := NewMemoryStore()
	if err := store.SaveSchedule(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveOccurrence(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	at := s.Spec.Recurrence.Start.Add(time.Hour)
	first, ok, err := store.ClaimDue(context.Background(), at, "worker-a", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first claim: %v", err)
	}
	second, ok, err := store.ClaimDue(context.Background(), at.Add(2*time.Minute), "worker-b", time.Minute)
	if err != nil || !ok || second.Fence <= first.Fence {
		t.Fatalf("reclaim: %+v ok=%v err=%v", second, ok, err)
	}
	current, valid, err := store.CheckDispatchFence(context.Background(), first.ID, "worker-a", first.Fence, at.Add(2*time.Minute))
	if err != nil || valid || current.Fence != second.Fence {
		t.Fatalf("stale fence accepted: current=%+v valid=%v err=%v", current, valid, err)
	}
}

func TestMalformedEmergencyStopFailsClosed(t *testing.T) {
	s := testSchedule(t)
	o := NewOccurrence(s, s.Spec.Recurrence.Start)
	store := NewMemoryStore()
	if err := store.SaveSchedule(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveOccurrence(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if err := store.SetEmergency(context.Background(), EmergencyStop{Active: true, Scope: "BAD", Actor: "usr_1", Reason: "incident", ChangedAt: time.Now()}); err == nil {
		t.Fatal("malformed emergency stop accepted")
	}
}
