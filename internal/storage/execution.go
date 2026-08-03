package storage

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/execution"
)

type CreateExecutionResult struct {
	Execution execution.Execution
	Created   bool
}

// ExecutionRepository is a persistence contract only. CreateExecution must
// enforce one execution per operation key and return the existing record for an
// exact request replay. UpdateExecution uses optimistic revision checks.
type ExecutionRepository interface {
	FindExecutionByID(context.Context, Scope, string) (execution.Execution, error)
	FindExecutionByRequestKey(context.Context, Scope, string, uint64) (execution.Execution, error)
	FindExecutionByOperationKey(context.Context, Scope, string, uint64) (execution.Execution, error)
	CreateExecution(context.Context, Scope, execution.Execution) (CreateExecutionResult, error)
	UpdateExecution(ctx context.Context, scope Scope, value execution.Execution, expectedRevision uint64) (execution.Execution, error)
}
