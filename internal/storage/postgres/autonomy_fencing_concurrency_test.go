package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/autonomy"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

type autonomyFixture struct {
	scope      storage.Scope
	grant      autonomy.Grant
	delegation autonomy.Delegation
	schedule   autonomy.Schedule
}

func createAutonomyFixture(t *testing.T) autonomyFixture {
	t.Helper()
	f := createBaseFixture(t, false)
	grant := autonomy.Grant{ID: unique("grant"), Version: 1, PrincipalUserID: f.scope.ActorID(), WalletBindingID: f.binding.BindingID(), Intent: autonomy.IntentPayroll, ExpiresAt: fixtureNow.Add(time.Hour), AggregateCapBaseUnits: "100"}
	if err := integrationStore.SaveAutonomyGrant(context.Background(), f.scope, grant); err != nil {
		t.Fatal(err)
	}
	delegation := autonomy.Delegation{ID: unique("delegation"), Version: 1, PrincipalUserID: f.scope.ActorID(), AgentID: unique("agent"), Capabilities: []autonomy.IntentType{autonomy.IntentPayroll}, ExpiresAt: fixtureNow.Add(time.Hour), NonTransitive: true}
	if err := integrationStore.SaveAutonomyDelegation(context.Background(), f.scope, delegation); err != nil {
		t.Fatal(err)
	}
	schedule := autonomy.Schedule{ID: unique("schedule"), Version: 1, Principal: autonomy.Principal{TenantID: f.scope.TenantID(), UserID: f.scope.ActorID(), AgentID: delegation.AgentID}, WalletBindingID: f.binding.BindingID(), WalletBindingVersion: f.binding.Version(), GrantID: grant.ID, GrantVersion: grant.Version, DelegationID: delegation.ID, DelegationVersion: delegation.Version, CreatedAt: fixtureNow, UpdatedAt: fixtureNow, Status: autonomy.ScheduleActive, Spec: autonomy.ScheduleSpec{Recurrence: autonomy.Recurrence{Frequency: autonomy.Daily, Start: fixtureNow.Add(-time.Minute), Location: "UTC"}, Missed: autonomy.MissedRunLatest, Concurrency: autonomy.ForbidOverlap, MaxRecipients: 1, Intent: autonomy.IntentPayroll, TemplateDigest: "sha256:typed"}}
	schedule.Digest = schedule.ComputeDigest()
	if err := integrationStore.SaveAutonomySchedule(context.Background(), f.scope, schedule); err != nil {
		t.Fatal(err)
	}
	return autonomyFixture{scope: f.scope, grant: grant, delegation: delegation, schedule: schedule}
}

func saveDueOccurrence(t *testing.T, f autonomyFixture, at time.Time) autonomy.Occurrence {
	t.Helper()
	o := autonomy.NewOccurrence(f.schedule, at)
	if err := integrationStore.SaveAutonomyOccurrence(context.Background(), f.scope, o); err != nil {
		t.Fatal(err)
	}
	return o
}

func TestPostgresAutonomyDispatchFenceIsScopedAndDurable(t *testing.T) {
	f := createAutonomyFixture(t)
	o := saveDueOccurrence(t, f, fixtureNow.Add(-time.Minute))
	claimed, ok, err := integrationStore.ClaimAutonomyDue(context.Background(), f.scope, fixtureNow, "worker-a", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %+v ok=%v err=%v", claimed, ok, err)
	}
	runtimeStore, err := NewRuntimeStore(integrationStore, f.scope)
	if err != nil {
		t.Fatal(err)
	}
	current, valid, err := runtimeStore.CheckDispatchFence(context.Background(), o.ID, "worker-a", claimed.Fence, fixtureNow.Add(time.Second))
	if err != nil || !valid || current.ID != o.ID || current.Fence != claimed.Fence {
		t.Fatalf("current fence: current=%+v valid=%v err=%v", current, valid, err)
	}
	if current, valid, err = runtimeStore.CheckDispatchFence(context.Background(), o.ID, "worker-b", claimed.Fence, fixtureNow.Add(time.Second)); err != nil || valid || current.Fence != claimed.Fence {
		t.Fatalf("wrong worker: current=%+v valid=%v err=%v", current, valid, err)
	}
	if current, valid, err = runtimeStore.CheckDispatchFence(context.Background(), o.ID, "worker-a", claimed.Fence, fixtureNow.Add(2*time.Minute)); err != nil || valid || current.Fence != claimed.Fence {
		t.Fatalf("expired lease: current=%+v valid=%v err=%v", current, valid, err)
	}
	wrongScope, err := storage.NewScope(f.scope.TenantID(), unique("different-user"), unique("request"), "")
	if err != nil {
		t.Fatal(err)
	}
	wrongRuntime, err := NewRuntimeStore(integrationStore, wrongScope)
	if err != nil {
		t.Fatal(err)
	}
	if _, valid, err := wrongRuntime.CheckDispatchFence(context.Background(), o.ID, "worker-a", claimed.Fence, fixtureNow.Add(time.Second)); err != nil || valid {
		t.Fatalf("wrong user scope accepted: valid=%v err=%v", valid, err)
	}
}

func TestPostgresAutonomyExpiredClaimReclaimsAndFences(t *testing.T) {
	f := createAutonomyFixture(t)
	o := saveDueOccurrence(t, f, fixtureNow.Add(-time.Minute))
	first, ok, err := integrationStore.ClaimAutonomyDue(context.Background(), f.scope, fixtureNow, "worker-a", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first claim: %+v ok=%v err=%v", first, ok, err)
	}
	second, ok, err := integrationStore.ClaimAutonomyDue(context.Background(), f.scope, fixtureNow.Add(2*time.Minute), "worker-b", time.Minute)
	if err != nil || !ok || second.ID != o.ID || second.Fence <= first.Fence || second.LeaseOwner != "worker-b" {
		t.Fatalf("reclaim: %+v ok=%v err=%v", second, ok, err)
	}
	if _, valid, err := integrationStore.CheckAutonomyDispatchFence(context.Background(), f.scope, first, "worker-a", first.Fence, fixtureNow.Add(2*time.Minute)); err != nil || valid {
		t.Fatalf("stale worker accepted: valid=%v err=%v", valid, err)
	}
	if current, valid, err := integrationStore.CheckAutonomyDispatchFence(context.Background(), f.scope, second, "worker-b", second.Fence, fixtureNow.Add(2*time.Minute+time.Second)); err != nil || !valid || current.Fence != second.Fence {
		t.Fatalf("new worker rejected: current=%+v valid=%v err=%v", current, valid, err)
	}
}

func TestPostgresAutonomyForbidOverlapSerializesSameSchedule(t *testing.T) {
	for run := 0; run < 5; run++ {
		f := createAutonomyFixture(t)
		saveDueOccurrence(t, f, fixtureNow.Add(-2*time.Minute))
		saveDueOccurrence(t, f, fixtureNow.Add(-time.Minute))
		var wg sync.WaitGroup
		results := make(chan bool, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(worker string) {
				defer wg.Done()
				_, ok, err := integrationStore.ClaimAutonomyDue(context.Background(), f.scope, fixtureNow, worker, time.Minute)
				if err != nil {
					t.Errorf("claim: %v", err)
				}
				results <- ok
			}(unique("worker"))
		}
		wg.Wait()
		close(results)
		winners := 0
		for ok := range results {
			if ok {
				winners++
			}
		}
		if winners != 1 {
			t.Fatalf("run %d winners=%d, want 1", run, winners)
		}
	}
}

func TestPostgresAutonomyDifferentSchedulesClaimConcurrently(t *testing.T) {
	f := createAutonomyFixture(t)
	for i := 0; i < 2; i++ {
		s := f.schedule
		s.ID = unique("independent-schedule")
		s.CreatedAt = fixtureNow
		s.UpdatedAt = fixtureNow
		s.Digest = s.ComputeDigest()
		if err := integrationStore.SaveAutonomySchedule(context.Background(), f.scope, s); err != nil {
			t.Fatal(err)
		}
		o := autonomy.NewOccurrence(s, fixtureNow.Add(-time.Minute))
		if err := integrationStore.SaveAutonomyOccurrence(context.Background(), f.scope, o); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(worker string) {
			defer wg.Done()
			_, ok, err := integrationStore.ClaimAutonomyDue(context.Background(), f.scope, fixtureNow, worker, time.Minute)
			if err != nil {
				t.Errorf("claim: %v", err)
			}
			results <- ok
		}(unique("worker"))
	}
	wg.Wait()
	close(results)
	winners := 0
	for ok := range results {
		if ok {
			winners++
		}
	}
	if winners != 2 {
		t.Fatalf("independent schedule winners=%d, want 2", winners)
	}
}
