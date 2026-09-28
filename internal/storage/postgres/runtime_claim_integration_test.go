package postgres

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/execution"
)

func TestExecutionWorkClaimIsExclusiveAndStaleLeaseCanBeRecovered(t *testing.T) {
	f := createBaseFixture(t, true)
	ctx := context.Background()
	now := fixtureNow.Add(time.Minute)

	first, acquired, err := integrationStore.ClaimExecutionWork(ctx, f.scope, f.execution.ExecutionID(), "worker-a", now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired || first.FencingToken != 1 || first.Execution.ExecutionID() != f.execution.ExecutionID() {
		t.Fatalf("first claim = %+v, acquired=%t", first, acquired)
	}

	if _, acquired, err = integrationStore.ClaimExecutionWork(ctx, f.scope, f.execution.ExecutionID(), "worker-b", now.Add(time.Second), 30*time.Second); err != nil || acquired {
		t.Fatalf("active duplicate claim acquired=%t, err=%v", acquired, err)
	}

	if _, err := integrationPool.Exec(ctx, `UPDATE execution_runtime_work SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE tenant_id=$1 AND execution_id=$2`, f.scope.TenantID(), f.execution.ExecutionID()); err != nil {
		t.Fatal(err)
	}
	second, acquired, err := integrationStore.ClaimExecutionWork(ctx, f.scope, f.execution.ExecutionID(), "worker-b", now.Add(31*time.Second), 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired || second.FencingToken != 2 || second.LeaseOwner != "worker-b" {
		t.Fatalf("recovered claim = %+v, acquired=%t", second, acquired)
	}

	if released, err := integrationStore.ReleaseExecutionWork(ctx, first, now.Add(32*time.Second)); err == nil || released {
		t.Fatalf("stale release = %t, err=%v", released, err)
	}
	if released, err := integrationStore.ReleaseExecutionWork(ctx, second, now.Add(32*time.Second)); err != nil || !released {
		t.Fatalf("current release = %t, err=%v", released, err)
	}
}

func TestTrackFSubmissionStartMarkerHasOneWinnerAndSurvivesReclaim(t *testing.T) {
	f := createBaseFixture(t, true)
	ctx := context.Background()
	now := fixtureNow.Add(3 * time.Minute)
	claim, acquired, err := integrationStore.ClaimExecutionWork(ctx, f.scope, f.execution.ExecutionID(), "track-f-owner", now, 30*time.Second)
	if err != nil || !acquired {
		t.Fatalf("claim acquired=%t err=%v", acquired, err)
	}

	var winners atomic.Int64
	var wait sync.WaitGroup
	errors := make(chan error, 16)
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, started, markErr := integrationStore.MarkSubmissionStarted(ctx, claim, now.Add(time.Second))
			if markErr != nil {
				errors <- markErr
				return
			}
			if started {
				winners.Add(1)
			}
		}()
	}
	wait.Wait()
	close(errors)
	for markErr := range errors {
		t.Errorf("mark submission started: %v", markErr)
	}
	if winners.Load() != 1 {
		t.Fatalf("submission-start winners = %d, want exactly 1", winners.Load())
	}

	if _, err := integrationPool.Exec(ctx, `UPDATE execution_runtime_work SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE tenant_id=$1 AND execution_id=$2`, f.scope.TenantID(), f.execution.ExecutionID()); err != nil {
		t.Fatal(err)
	}
	recovered, acquired, err := integrationStore.ClaimExecutionWork(ctx, f.scope, f.execution.ExecutionID(), "track-f-recovery", now.Add(31*time.Second), 30*time.Second)
	if err != nil || !acquired {
		t.Fatalf("recovery claim acquired=%t err=%v", acquired, err)
	}
	if !recovered.SubmissionStarted || recovered.FencingToken != claim.FencingToken+1 {
		t.Fatalf("recovered marker/fence = started:%t fence:%d, prior:%d", recovered.SubmissionStarted, recovered.FencingToken, claim.FencingToken)
	}
	if _, started, err := integrationStore.MarkSubmissionStarted(ctx, recovered, now.Add(32*time.Second)); err != nil || started {
		t.Fatalf("recovery must not win another first submission: started=%t err=%v", started, err)
	}
}

func TestClaimNextExecutionWorkDerivesTrustedPersistedScope(t *testing.T) {
	f := createBaseFixture(t, true)
	claim, acquired, err := integrationStore.ClaimNextExecutionWork(context.Background(), "worker-next", fixtureNow.Add(2*time.Minute), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("no work claimed")
	}
	if claim.Execution.ExecutionID() == "" || claim.Scope.TenantID() == "" || claim.Scope.ActorID() == "" || claim.Scope.RequestID() == "" {
		t.Fatalf("claim scope is incomplete: %+v", claim)
	}
	if claim.Execution.ExecutionID() == f.execution.ExecutionID() && (claim.Scope.TenantID() != f.scope.TenantID() || claim.Scope.ActorID() != f.scope.ActorID()) {
		t.Fatalf("claim scope does not match persisted owner: %+v", claim.Scope)
	}
	if _, err := integrationStore.ReleaseExecutionWork(context.Background(), claim, fixtureNow.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
}

func TestDirectClaimRejectsTerminalExecution(t *testing.T) {
	f := createBaseFixture(t, true)
	value := f.execution
	for index, status := range []execution.Status{execution.StatusAuthorized, execution.StatusQueued, execution.StatusExecuting} {
		next, err := value.Transition(status, fixtureNow.Add(time.Duration(8+index)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = integrationStore.UpdateExecution(context.Background(), f.scope, next, value.Revision()); err != nil {
			t.Fatal(err)
		}
		value = next
	}
	failed, err := value.Fail(execution.Failure{Code: "PROVEN_TERMINAL_FAILURE", Eligibility: execution.RecoveryTerminal}, fixtureNow.Add(12*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.UpdateExecution(context.Background(), f.scope, failed, value.Revision()); err != nil {
		t.Fatal(err)
	}
	if _, acquired, err := integrationStore.ClaimExecutionWork(context.Background(), f.scope, failed.ExecutionID(), "worker-terminal", fixtureNow.Add(time.Minute), time.Minute); err != nil || acquired {
		t.Fatalf("terminal claim acquired=%t, err=%v", acquired, err)
	}
}
