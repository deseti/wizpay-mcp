package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/autonomy"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

func TestAutonomyClaimNextPreservesSameTenantOwner(t *testing.T) {
	ctx := context.Background()
	tenant := unique("shared-tenant")
	if _, err := integrationStore.CreateTenant(ctx, storage.Tenant{TenantID: tenant, CreatedAt: fixtureNow}); err != nil {
		t.Fatal(err)
	}
	type owner struct {
		scope storage.Scope
		user  string
	}
	owners := make([]owner, 2)
	for i := range owners {
		user := unique("owner")
		scope, err := storage.NewScope(tenant, user, unique("request"), "")
		if err != nil {
			t.Fatal(err)
		}
		owners[i] = owner{scope: scope, user: user}
		grant := autonomy.Grant{ID: unique("grant"), Version: 1, PrincipalUserID: user, WalletBindingID: unique("wallet"), Intent: autonomy.IntentPayroll, ExpiresAt: fixtureNow.Add(time.Hour), AggregateCapBaseUnits: "100"}
		if err := integrationStore.SaveAutonomyGrant(ctx, scope, grant); err != nil {
			t.Fatal(err)
		}
		delegation := autonomy.Delegation{ID: unique("delegation"), Version: 1, PrincipalUserID: user, AgentID: "agent_" + user, Capabilities: []autonomy.IntentType{autonomy.IntentPayroll}, ExpiresAt: fixtureNow.Add(time.Hour), NonTransitive: true}
		if err := integrationStore.SaveAutonomyDelegation(ctx, scope, delegation); err != nil {
			t.Fatal(err)
		}
		schedule := autonomy.Schedule{ID: unique("schedule"), Version: 1, Principal: autonomy.Principal{TenantID: tenant, UserID: user, AgentID: delegation.AgentID}, WalletBindingID: grant.WalletBindingID, WalletBindingVersion: 1, GrantID: grant.ID, GrantVersion: grant.Version, DelegationID: delegation.ID, DelegationVersion: 1, CreatedAt: fixtureNow, UpdatedAt: fixtureNow, Status: autonomy.ScheduleActive, Spec: autonomy.ScheduleSpec{Recurrence: autonomy.Recurrence{Frequency: autonomy.Once, Start: fixtureNow.Add(-time.Minute), Location: "UTC"}, Missed: autonomy.MissedRunLatest, Concurrency: autonomy.ForbidOverlap, MaxRecipients: 1, Intent: autonomy.IntentPayroll, TemplateDigest: "sha256:typed"}}
		schedule.Digest = schedule.ComputeDigest()
		if err := integrationStore.SaveAutonomySchedule(ctx, scope, schedule); err != nil {
			t.Fatal(err)
		}
		if err := integrationStore.SaveAutonomyOccurrence(ctx, scope, autonomy.NewOccurrence(schedule, schedule.Spec.Recurrence.Start)); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	results := make(chan string, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scope, _, ok, err := integrationStore.ClaimNextAutonomyDue(ctx, "isolation-worker", fixtureNow, time.Minute)
			if err != nil {
				t.Error(err)
				return
			}
			if ok {
				results <- scope.ActorID()
			}
		}()
	}
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for user := range results {
		seen[user] = true
		if user != owners[0].user && user != owners[1].user {
			t.Fatalf("claimed scope owner %q is not an occurrence owner", user)
		}
	}
	if len(seen) != 2 {
		t.Fatalf("claim owners=%v, want both users", seen)
	}
}
