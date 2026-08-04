package wiring

import (
	"context"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/audit"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/execution/runtime"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

// stubRuntimeStore satisfies RuntimeStore for assembly tests. BuildWorker only
// checks that persistence is non-nil; construction of the service and worker
// never calls these methods, so returning zero values is sufficient and keeps
// the test honest that no persistence I/O happens during wiring.
type stubRuntimeStore struct{}

func (stubRuntimeStore) FindExecutionByID(context.Context, storage.Scope, string) (execution.Execution, error) {
	return execution.Execution{}, nil
}

func (stubRuntimeStore) FindExecutionByRequestKey(context.Context, storage.Scope, string, uint64) (execution.Execution, error) {
	return execution.Execution{}, nil
}

func (stubRuntimeStore) FindExecutionByOperationKey(context.Context, storage.Scope, string, uint64) (execution.Execution, error) {
	return execution.Execution{}, nil
}

func (stubRuntimeStore) CreateExecution(context.Context, storage.Scope, execution.Execution) (storage.CreateExecutionResult, error) {
	return storage.CreateExecutionResult{}, nil
}

func (stubRuntimeStore) UpdateExecution(context.Context, storage.Scope, execution.Execution, uint64) (execution.Execution, error) {
	return execution.Execution{}, nil
}

func (stubRuntimeStore) AppendVerificationEvidence(context.Context, storage.Scope, execution.Result) error {
	return nil
}

func (stubRuntimeStore) FindVerificationEvidence(context.Context, storage.Scope, string) ([]execution.Result, error) {
	return nil, nil
}

func (stubRuntimeStore) ClaimExecutionWork(context.Context, storage.Scope, string, string, time.Time, time.Duration) (storage.ExecutionClaim, bool, error) {
	return storage.ExecutionClaim{}, false, nil
}

func (stubRuntimeStore) ClaimNextExecutionWork(context.Context, string, time.Time, time.Duration) (storage.ExecutionClaim, bool, error) {
	return storage.ExecutionClaim{}, false, nil
}

func (stubRuntimeStore) MarkSubmissionStarted(context.Context, storage.ExecutionClaim, time.Time) (storage.ExecutionClaim, bool, error) {
	return storage.ExecutionClaim{}, false, nil
}

func (stubRuntimeStore) ResetSubmissionStarted(context.Context, storage.ExecutionClaim, time.Time) (storage.ExecutionClaim, bool, error) {
	return storage.ExecutionClaim{}, false, nil
}

func (stubRuntimeStore) UpdateClaimedExecution(context.Context, storage.ExecutionClaim, execution.Execution, uint64, audit.Record, time.Time) (execution.Execution, error) {
	return execution.Execution{}, nil
}

func (stubRuntimeStore) PersistClaimedObservation(context.Context, storage.ExecutionClaim, execution.Result, execution.Execution, uint64, audit.Record, time.Time) (execution.Execution, error) {
	return execution.Execution{}, nil
}

func (stubRuntimeStore) PersistClaimedEvidence(context.Context, storage.ExecutionClaim, execution.Result, execution.Execution, uint64, audit.Record, time.Time) (execution.Execution, error) {
	return execution.Execution{}, nil
}

func (stubRuntimeStore) ReleaseExecutionWork(context.Context, storage.ExecutionClaim, time.Time) (bool, error) {
	return false, nil
}

var _ RuntimeStore = stubRuntimeStore{}

func workerConfig() runtime.Config {
	return runtime.Config{
		WorkerID:      "worker-test",
		LeaseDuration: 30 * time.Second,
		RetryInterval: 5 * time.Second,
	}
}

func noopSleep(ctx context.Context, _ time.Duration) error {
	return ctx.Err()
}

func TestBuildWorkerRequiresPersistence(t *testing.T) {
	if _, _, err := BuildWorker(configuredPlane(t), nil, workerConfig(), fixedClock(), noopSleep); err == nil {
		t.Fatalf("BuildWorker must reject a nil store")
	}
}

func TestBuildWorkerRequiresClockAndSleeper(t *testing.T) {
	if _, _, err := BuildWorker(configuredPlane(t), stubRuntimeStore{}, workerConfig(), nil, noopSleep); err == nil {
		t.Fatalf("BuildWorker must reject a nil clock")
	}
	if _, _, err := BuildWorker(configuredPlane(t), stubRuntimeStore{}, workerConfig(), fixedClock(), nil); err == nil {
		t.Fatalf("BuildWorker must reject a nil sleeper")
	}
}

func TestBuildWorkerUnconfiguredWithoutAdapter(t *testing.T) {
	// The Phase 11 state: no adapter on the plane means no worker is built, so
	// the process idles and no execution — and no financial transaction — runs.
	worker, configured, err := BuildWorker(unconfiguredPlane(t), stubRuntimeStore{}, workerConfig(), fixedClock(), noopSleep)
	if err != nil {
		t.Fatalf("BuildWorker: %v", err)
	}
	if configured {
		t.Fatalf("an unconfigured plane must not produce a worker")
	}
	if worker != nil {
		t.Fatalf("no worker may be returned without an adapter")
	}
}

func TestBuildWorkerConfigured(t *testing.T) {
	worker, configured, err := BuildWorker(configuredPlane(t), stubRuntimeStore{}, workerConfig(), fixedClock(), noopSleep)
	if err != nil {
		t.Fatalf("BuildWorker: %v", err)
	}
	if !configured {
		t.Fatalf("a fully configured plane must produce a worker")
	}
	if worker == nil {
		t.Fatalf("expected a worker when configured")
	}
}
