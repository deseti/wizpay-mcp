package storage

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/audit"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

// AtomicRepository captures only invariants that require multiple durable
// records to commit or roll back together.
type AtomicRepository interface {
	CreateIntentWithAudit(context.Context, Scope, intents.Intent, audit.Record) (CreateIntentResult, error)
	CreateApprovalWithAudit(context.Context, Scope, approvals.Approval, audit.Record) (CreateApprovalResult, error)
	ConsumeApprovalAndCreateExecution(context.Context, Scope, approvals.Approval, uint64, execution.Execution, audit.Record) (approvals.Approval, CreateExecutionResult, error)
	UpdateExecutionWithAudit(context.Context, Scope, execution.Execution, uint64, audit.Record) (execution.Execution, error)
	AppendEvidenceAndVerify(context.Context, Scope, execution.Result, execution.Execution, uint64, audit.Record) (execution.Execution, error)
}
