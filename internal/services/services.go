// Package services defines the application-service ports used by transport
// layers. Implementations remain responsible for identity resolution,
// authorization, persistence, and domain orchestration.
package services

import (
	"context"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/autonomy"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
)

type CreateIntentCommand struct {
	ClientRequestID string
	Nonce           string
	WalletBindingID string
	Type            intents.Type
	Financial       intents.FinancialParameters
	Route           intents.Route
	Deadline        time.Time
	PolicyReference string
}

type IntentService interface {
	CreateIntent(context.Context, CreateIntentCommand) (intents.Intent, error)
	GetIntent(context.Context, string) (intents.Intent, error)
}

type ApprovalService interface {
	RequestApproval(context.Context, string) (approvals.Approval, error)
	GetApproval(context.Context, string) (approvals.Approval, error)
}

type PolicyService interface {
	EvaluatePolicy(context.Context, string, string, uint64, policies.EvaluationStage) (policies.Result, error)
}

type ExecutionService interface {
	PrepareExecution(context.Context, string, string, string, uint64) (execution.Request, error)
}

// AutonomyService is the application boundary for Phase 13 read/control
// surfaces. Implementations must resolve authenticated scope themselves;
// callers cannot supply a Principal or tenant as authority.
type AutonomyService interface {
	ListSchedules(context.Context) ([]autonomy.Schedule, error)
	GetSchedule(context.Context, string, uint64) (autonomy.Schedule, error)
	SimulateOccurrence(context.Context, string, uint64, time.Time) (autonomy.Decision, error)
	CreateSchedule(context.Context, autonomy.Schedule) (autonomy.Schedule, error)
	SetScheduleStatus(context.Context, string, uint64, autonomy.ScheduleStatus) (autonomy.Schedule, error)
	SetEmergencyStop(context.Context, autonomy.EmergencyStop) (autonomy.EmergencyStop, error)
}

// Bundle contains only domain orchestration ports. It intentionally has no
// adapter, signer, provider, transaction, or blockchain dependency.
type Bundle struct {
	Intents    IntentService
	Approvals  ApprovalService
	Policies   PolicyService
	Executions ExecutionService
}
