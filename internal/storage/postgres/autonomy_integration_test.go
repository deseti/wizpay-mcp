package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/autonomy"
)

func TestAutonomyPersistenceClaimAndReservation(t *testing.T) {
	f := createBaseFixture(t, false)
	ctx := context.Background()
	grant := autonomy.Grant{ID: unique("grant"), Version: 1, PrincipalUserID: f.scope.ActorID(), WalletBindingID: f.binding.BindingID(), Intent: autonomy.IntentPayroll, ExpiresAt: fixtureNow.Add(time.Hour), AggregateCapBaseUnits: "50", RollingWindowCapBaseUnits: "50", RollingWindow: time.Hour}
	if err := integrationStore.SaveAutonomyGrant(ctx, f.scope, grant); err != nil {
		t.Fatal(err)
	}
	if err := integrationStore.SaveAutonomyGrant(ctx, f.scope, grant); err != nil {
		t.Fatal("grant replay: ", err)
	}
	conflictingGrant := grant
	conflictingGrant.AggregateCapBaseUnits = "49"
	if err := integrationStore.SaveAutonomyGrant(ctx, f.scope, conflictingGrant); err == nil {
		t.Fatal("grant immutable mismatch accepted")
	}
	delegation := autonomy.Delegation{ID: unique("delegation"), Version: 1, PrincipalUserID: f.scope.ActorID(), AgentID: "agent_1", Capabilities: []autonomy.IntentType{autonomy.IntentPayroll}, ExpiresAt: fixtureNow.Add(time.Hour), NonTransitive: true}
	if err := integrationStore.SaveAutonomyDelegation(ctx, f.scope, delegation); err != nil {
		t.Fatal(err)
	}
	if err := integrationStore.SaveAutonomyDelegation(ctx, f.scope, delegation); err != nil {
		t.Fatal("delegation replay: ", err)
	}
	conflictingDelegation := delegation
	conflictingDelegation.AgentID = "other-agent"
	if err := integrationStore.SaveAutonomyDelegation(ctx, f.scope, conflictingDelegation); err == nil {
		t.Fatal("delegation immutable mismatch accepted")
	}
	schedule := autonomy.Schedule{ID: unique("schedule"), Version: 1, Principal: autonomy.Principal{TenantID: f.scope.TenantID(), UserID: f.scope.ActorID(), AgentID: delegation.AgentID}, WalletBindingID: f.binding.BindingID(), WalletBindingVersion: f.binding.Version(), GrantID: grant.ID, GrantVersion: grant.Version, DelegationID: delegation.ID, DelegationVersion: delegation.Version, CreatedAt: fixtureNow, UpdatedAt: fixtureNow, Status: autonomy.ScheduleActive, Spec: autonomy.ScheduleSpec{Recurrence: autonomy.Recurrence{Frequency: autonomy.Daily, Start: fixtureNow.Add(time.Minute), Location: "UTC"}, Missed: autonomy.MissedRunLatest, Concurrency: autonomy.ForbidOverlap, MaxRecipients: 10, Intent: autonomy.IntentPayroll, TemplateDigest: "sha256:typed"}}
	schedule.Digest = schedule.ComputeDigest()
	if err := integrationStore.SaveAutonomySchedule(ctx, f.scope, schedule); err != nil {
		t.Fatal(err)
	}
	loaded, err := integrationStore.LoadAutonomySchedule(ctx, f.scope, schedule.ID, schedule.Version)
	if err != nil || loaded.Digest != schedule.Digest {
		t.Fatalf("schedule restore: %v", err)
	}
	occurrence := autonomy.NewOccurrence(schedule, schedule.Spec.Recurrence.Start)
	if err := integrationStore.SaveAutonomyOccurrence(ctx, f.scope, occurrence); err != nil {
		t.Fatal(err)
	}
	conflictingOccurrence := occurrence
	conflictingOccurrence.ScheduleDigest = "sha256:other"
	if err := integrationStore.SaveAutonomyOccurrence(ctx, f.scope, conflictingOccurrence); err == nil {
		t.Fatal("occurrence immutable mismatch accepted")
	}
	if err := integrationStore.SaveAutonomyOccurrence(ctx, f.scope, occurrence); err != nil {
		t.Fatal("idempotent occurrence replay: ", err)
	}
	claimed, ok, err := integrationStore.ClaimAutonomyDue(ctx, f.scope, fixtureNow.Add(time.Hour), "worker-1", time.Minute)
	if err != nil || !ok || claimed.ID != occurrence.ID {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	second := autonomy.NewOccurrence(schedule, schedule.Spec.Recurrence.Start.Add(24*time.Hour))
	if err := integrationStore.SaveAutonomyOccurrence(ctx, f.scope, second); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for _, occurrenceID := range []string{occurrence.ID, second.ID} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			allowed, reserveErr := integrationStore.ReserveAutonomySpend(ctx, f.scope, grant, id, "50", fixtureNow)
			results <- reserveErr == nil && allowed
		}(occurrenceID)
	}
	wg.Wait()
	close(results)
	winners := 0
	for allowed := range results {
		if allowed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("reservation winners=%d, want 1", winners)
	}
	stop := autonomy.EmergencyStop{Active: true, Scope: "TENANT", Actor: f.scope.ActorID(), Reason: "test", ChangedAt: fixtureNow}
	if err := integrationStore.SetAutonomyEmergencyStop(ctx, f.scope, stop); err != nil {
		t.Fatal(err)
	}
	restored, err := integrationStore.LoadAutonomyEmergencyStop(ctx, f.scope)
	if err != nil || !restored.Active {
		t.Fatalf("emergency stop restore: %v", err)
	}
}
