package storage

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/policies"
)

type PolicyEvaluationRepository interface {
	CreatePolicyEvaluation(context.Context, Scope, policies.Result) (policies.Result, error)
	FindPolicyEvaluation(context.Context, Scope, string) (policies.Result, error)
}

type VerificationEvidenceRepository interface {
	AppendVerificationEvidence(context.Context, Scope, execution.Result) error
	FindVerificationEvidence(context.Context, Scope, string) ([]execution.Result, error)
}
