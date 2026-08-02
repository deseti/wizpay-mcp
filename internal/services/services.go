// Package services defines the application-service ports used by transport
// layers. Implementations remain responsible for identity resolution,
// authorization, persistence, and domain orchestration.
package services

import (
	"context"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
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

// Bundle contains only domain orchestration ports. It intentionally has no
// adapter, signer, provider, transaction, or blockchain dependency.
type Bundle struct {
	Intents    IntentService
	Approvals  ApprovalService
	Policies   PolicyService
	Executions ExecutionService
}
