package postgres

import (
	"bytes"
	"context"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deseti/wizpay-mcp/internal/autonomy"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

func newIsolatedAutonomyStore(t *testing.T) *Store {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	database := unique("autonomy_claim_db")
	quotedDatabase := pgx.Identifier{database}.Sanitize()
	if _, err := integrationPool.Exec(ctx, "CREATE DATABASE "+quotedDatabase); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(integrationURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + database
	pool, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	store, err := NewStore(pool, 10*time.Second, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		store.Close()
		dropCtx, dropCancel := context.WithTimeout(context.Background(), time.Minute)
		defer dropCancel()
		if _, err := integrationPool.Exec(dropCtx, "DROP DATABASE "+quotedDatabase+" WITH (FORCE)"); err != nil {
			t.Errorf("drop isolated database: %v", err)
		}
	})
	return store
}

func createAutonomyFixtureInStore(t *testing.T, store *Store, tenant, user, scheduleID string) autonomyFixture {
	t.Helper()
	ctx := context.Background()
	if _, err := store.CreateTenant(ctx, storage.Tenant{TenantID: tenant, CreatedAt: fixtureNow}); err != nil {
		t.Fatal(err)
	}
	scope, err := storage.NewScope(tenant, user, unique("request"), "")
	if err != nil {
		t.Fatal(err)
	}
	grant := autonomy.Grant{ID: unique("grant"), Version: 1, PrincipalUserID: user, WalletBindingID: unique("wallet"), Intent: autonomy.IntentPayroll, ExpiresAt: fixtureNow.Add(time.Hour), AggregateCapBaseUnits: "100"}
	if err := store.SaveAutonomyGrant(ctx, scope, grant); err != nil {
		t.Fatal(err)
	}
	delegation := autonomy.Delegation{ID: unique("delegation"), Version: 1, PrincipalUserID: user, AgentID: unique("agent"), Capabilities: []autonomy.IntentType{autonomy.IntentPayroll}, ExpiresAt: fixtureNow.Add(time.Hour), NonTransitive: true}
	if err := store.SaveAutonomyDelegation(ctx, scope, delegation); err != nil {
		t.Fatal(err)
	}
	schedule := autonomy.Schedule{
		ID: scheduleID, Version: 1,
		Principal:            autonomy.Principal{TenantID: tenant, UserID: user, AgentID: delegation.AgentID},
		WalletBindingID:      grant.WalletBindingID,
		WalletBindingVersion: 1,
		GrantID:              grant.ID,
		GrantVersion:         grant.Version,
		DelegationID:         delegation.ID,
		DelegationVersion:    delegation.Version,
		CreatedAt:            fixtureNow,
		UpdatedAt:            fixtureNow,
		Status:               autonomy.ScheduleActive,
		Spec: autonomy.ScheduleSpec{
			Recurrence:     autonomy.Recurrence{Frequency: autonomy.Daily, Start: fixtureNow.Add(-10 * time.Minute), Location: "UTC"},
			Missed:         autonomy.MissedRunLatest,
			Concurrency:    autonomy.ForbidOverlap,
			MaxRecipients:  1,
			Intent:         autonomy.IntentPayroll,
			TemplateDigest: "sha256:typed",
		},
	}
	schedule.Digest = schedule.ComputeDigest()
	if err := store.SaveAutonomySchedule(ctx, scope, schedule); err != nil {
		t.Fatal(err)
	}
	return autonomyFixture{scope: scope, grant: grant, delegation: delegation, schedule: schedule}
}

func saveOccurrenceInStore(t *testing.T, store *Store, fixture autonomyFixture, occurrence autonomy.Occurrence) {
	t.Helper()
	if err := store.SaveAutonomyOccurrence(context.Background(), fixture.scope, occurrence); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresClaimNextAutonomyDueSerializesSameSchedule(t *testing.T) {
	store := newIsolatedAutonomyStore(t)
	fixture := createAutonomyFixtureInStore(t, store, unique("tenant"), unique("user"), unique("schedule"))
	first := autonomy.NewOccurrence(fixture.schedule, fixtureNow.Add(-2*time.Minute))
	second := autonomy.NewOccurrence(fixture.schedule, fixtureNow.Add(-time.Minute))
	saveOccurrenceInStore(t, store, fixture, first)
	saveOccurrenceInStore(t, store, fixture, second)

	type result struct {
		occurrence autonomy.Occurrence
		ok         bool
		err        error
	}
	var workers sync.WaitGroup
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func(worker string) {
			defer workers.Done()
			ready.Done()
			<-start
			_, occurrence, ok, err := store.ClaimNextAutonomyDue(context.Background(), worker, fixtureNow, time.Minute)
			results <- result{occurrence: occurrence, ok: ok, err: err}
		}(unique("worker"))
	}
	ready.Wait()
	close(start)
	workers.Wait()
	close(results)

	winners := 0
	winnerID := ""
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.ok {
			winners++
			winnerID = result.occurrence.ID
		}
	}
	if winners != 1 {
		t.Fatalf("winners=%d, want 1", winners)
	}
	loserID := first.ID
	if winnerID == first.ID {
		loserID = second.ID
	}
	var loserStatus string
	if err := store.pool.QueryRow(context.Background(), `SELECT status FROM autonomy_occurrences WHERE tenant_id=$1 AND occurrence_id=$2`, fixture.scope.TenantID(), loserID).Scan(&loserStatus); err != nil {
		t.Fatal(err)
	}
	if loserStatus != string(autonomy.OccurrenceDue) {
		t.Fatalf("losing sibling status=%s, want DUE", loserStatus)
	}
}

func TestPostgresClaimNextSkipsBlockedEarlierSchedule(t *testing.T) {
	store := newIsolatedAutonomyStore(t)
	blocked := createAutonomyFixtureInStore(t, store, unique("tenant-a"), unique("user-a"), unique("schedule-a"))
	blockedDue := autonomy.NewOccurrence(blocked.schedule, fixtureNow.Add(-5*time.Minute))
	active := autonomy.NewOccurrence(blocked.schedule, fixtureNow.Add(-4*time.Minute))
	active.Status = autonomy.OccurrenceClaimed
	active.LeaseOwner = "active-worker"
	active.LeaseUntil = fixtureNow.Add(time.Minute)
	saveOccurrenceInStore(t, store, blocked, blockedDue)
	saveOccurrenceInStore(t, store, blocked, active)

	runnable := createAutonomyFixtureInStore(t, store, unique("tenant-b"), unique("user-b"), unique("schedule-b"))
	runnableOccurrence := autonomy.NewOccurrence(runnable.schedule, fixtureNow.Add(-3*time.Minute))
	saveOccurrenceInStore(t, store, runnable, runnableOccurrence)
	later := createAutonomyFixtureInStore(t, store, unique("tenant-c"), unique("user-c"), unique("schedule-c"))
	saveOccurrenceInStore(t, store, later, autonomy.NewOccurrence(later.schedule, fixtureNow.Add(-2*time.Minute)))

	scope, claimed, ok, err := store.ClaimNextAutonomyDue(context.Background(), "worker", fixtureNow, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: occurrence=%+v ok=%v err=%v", claimed, ok, err)
	}
	if claimed.ID != runnableOccurrence.ID || scope.TenantID() != runnable.scope.TenantID() {
		t.Fatalf("claimed tenant=%s occurrence=%s, want tenant=%s occurrence=%s", scope.TenantID(), claimed.ID, runnable.scope.TenantID(), runnableOccurrence.ID)
	}
}

func TestPostgresClaimNextReclaimsExpiredClaim(t *testing.T) {
	store := newIsolatedAutonomyStore(t)
	fixture := createAutonomyFixtureInStore(t, store, unique("tenant"), unique("user"), unique("schedule"))
	expired := autonomy.NewOccurrence(fixture.schedule, fixtureNow.Add(-time.Minute))
	expired.Status = autonomy.OccurrenceClaimed
	expired.LeaseOwner = "stale-worker"
	expired.LeaseUntil = fixtureNow.Add(-time.Second)
	saveOccurrenceInStore(t, store, fixture, expired)

	_, claimed, ok, err := store.ClaimNextAutonomyDue(context.Background(), "new-worker", fixtureNow, time.Minute)
	if err != nil || !ok || claimed.ID != expired.ID || claimed.LeaseOwner != "new-worker" || claimed.Fence <= expired.Fence {
		t.Fatalf("reclaim: occurrence=%+v ok=%v err=%v", claimed, ok, err)
	}
}

func TestPostgresClaimNextUsesOccurrenceIDAsExactTiebreak(t *testing.T) {
	store := newIsolatedAutonomyStore(t)
	at := fixtureNow.Add(-time.Minute)

	var lowestSchedule, lowestOccurrenceID string
	var highestSchedule, highestOccurrenceID string
	for i := 0; i < 32; i++ {
		scheduleID := unique("ordering-schedule")
		placeholder := autonomy.Schedule{ID: scheduleID, Version: 1}
		occurrenceID := autonomy.NewOccurrence(placeholder, at).ID
		if lowestOccurrenceID == "" || occurrenceID < lowestOccurrenceID {
			lowestSchedule, lowestOccurrenceID = scheduleID, occurrenceID
		}
		if highestOccurrenceID == "" || occurrenceID > highestOccurrenceID {
			highestSchedule, highestOccurrenceID = scheduleID, occurrenceID
		}
	}
	if lowestOccurrenceID >= highestOccurrenceID {
		t.Fatal("failed to construct distinct occurrence ordering")
	}

	// The lexically earlier tenant owns the higher occurrence ID. Ordering by
	// tenant or schedule would therefore disagree with the required tiebreak.
	earlierTenant := "tenant-a-" + unique("ordering")
	laterTenant := "tenant-z-" + unique("ordering")
	high := createAutonomyFixtureInStore(t, store, earlierTenant, unique("user-a"), highestSchedule)
	low := createAutonomyFixtureInStore(t, store, laterTenant, unique("user-z"), lowestSchedule)
	highOccurrence := autonomy.NewOccurrence(high.schedule, at)
	lowOccurrence := autonomy.NewOccurrence(low.schedule, at)
	saveOccurrenceInStore(t, store, high, highOccurrence)
	saveOccurrenceInStore(t, store, low, lowOccurrence)
	if !(earlierTenant < laterTenant && lowOccurrence.ID < highOccurrence.ID) {
		t.Fatalf("test setup does not conflict: tenants=%q/%q occurrence IDs=%q/%q", earlierTenant, laterTenant, highOccurrence.ID, lowOccurrence.ID)
	}

	scope, claimed, ok, err := store.ClaimNextAutonomyDue(context.Background(), "worker", fixtureNow, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: occurrence=%+v ok=%v err=%v", claimed, ok, err)
	}
	if claimed.ID != lowOccurrence.ID || scope.TenantID() != laterTenant {
		t.Fatalf("claimed tenant=%q occurrence=%q, want tenant=%q occurrence=%q", scope.TenantID(), claimed.ID, laterTenant, lowOccurrence.ID)
	}
	if strings.Compare(claimed.ID, highOccurrence.ID) >= 0 {
		t.Fatalf("claimed occurrence ID %q is not lower than %q", claimed.ID, highOccurrence.ID)
	}
}
