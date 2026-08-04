package storage

import (
	"context"
	"time"

	"github.com/deseti/wizpay-mcp/internal/audit"
	"github.com/deseti/wizpay-mcp/internal/execution"
)

// ExecutionClaim is a database-backed lease with a monotonically increasing
// fencing token. Runtime writes must present the current claim.
type ExecutionClaim struct {
	Scope             Scope
	Execution         execution.Execution
	LeaseOwner        string
	FencingToken      uint64
	LeaseExpiresAt    time.Time
	SubmissionStarted bool
}

type ExecutionRuntimeRepository interface {
	ClaimExecutionWork(context.Context, Scope, string, string, time.Time, time.Duration) (ExecutionClaim, bool, error)
	ClaimNextExecutionWork(context.Context, string, time.Time, time.Duration) (ExecutionClaim, bool, error)
	MarkSubmissionStarted(context.Context, ExecutionClaim, time.Time) (ExecutionClaim, bool, error)
	ResetSubmissionStarted(context.Context, ExecutionClaim, time.Time) (ExecutionClaim, bool, error)
	UpdateClaimedExecution(context.Context, ExecutionClaim, execution.Execution, uint64, audit.Record, time.Time) (execution.Execution, error)
	PersistClaimedObservation(context.Context, ExecutionClaim, execution.Result, execution.Execution, uint64, audit.Record, time.Time) (execution.Execution, error)
	PersistClaimedEvidence(context.Context, ExecutionClaim, execution.Result, execution.Execution, uint64, audit.Record, time.Time) (execution.Execution, error)
	ReleaseExecutionWork(context.Context, ExecutionClaim, time.Time) (bool, error)
}
