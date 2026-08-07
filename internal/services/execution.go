package services

import (
	"context"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

// PersistedExecutionService prepares authorization references only. It does
// not submit, sign, or call an execution/provider runtime.
type PersistedExecutionService struct {
	Intents     storage.IntentRepository
	Approvals   storage.ApprovalRepository
	Policies    storage.PolicyRepository
	Evaluations storage.PolicyEvaluationRepository
	Executions  storage.ExecutionRepository
	Wallets     storage.WalletBindingRepository
	Authorizer  auth.Authorizer
	Now         func() time.Time
}

func (s *PersistedExecutionService) PrepareExecution(ctx context.Context, intentID, approvalID, policyID string, policyVersion uint64) (execution.Request, error) {
	if s == nil || s.Authorizer == nil || s.Intents == nil || s.Approvals == nil || s.Policies == nil || s.Evaluations == nil || s.Executions == nil || s.Wallets == nil || s.Now == nil {
		return execution.Request{}, fmt.Errorf("execution service is not configured")
	}
	request, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		return execution.Request{}, err
	}
	if err := s.Authorizer.Authorize(ctx, auth.AuthorizationInput{Request: request, Permission: auth.PermissionPrepareExecution}); err != nil {
		return execution.Request{}, err
	}
	scope, err := requestauth.StorageScopeFromContext(ctx)
	if err != nil {
		return execution.Request{}, err
	}
	intent, err := s.Intents.FindIntentByID(ctx, scope, intentID)
	if err != nil {
		return execution.Request{}, err
	}
	approval, err := s.Approvals.FindApprovalByID(ctx, scope, approvalID)
	if err != nil {
		return execution.Request{}, err
	}
	policy, err := s.Policies.FindPolicyByID(ctx, scope, policyID, policyVersion)
	if err != nil {
		return execution.Request{}, err
	}
	binding, err := s.Wallets.FindBindingByID(ctx, scope, intent.Ownership().WalletBindingID)
	if err != nil {
		return execution.Request{}, err
	}
	identity, err := auth.NewIdentityContext(request.Identity(), request.Metadata())
	if err != nil {
		return execution.Request{}, err
	}
	result, err := policies.Evaluate(policy, intent, identity, binding, s.Now().UTC())
	if err != nil {
		return execution.Request{}, err
	}
	result, err = s.Evaluations.CreatePolicyEvaluation(ctx, scope, result)
	if err != nil {
		return execution.Request{}, err
	}
	requestValue, err := execution.NewRequest(intent, approval, result, s.Now().UTC())
	if err != nil {
		return execution.Request{}, err
	}
	executionValue, err := execution.New(requestValue)
	if err != nil {
		return execution.Request{}, err
	}
	created, err := s.Executions.CreateExecution(ctx, scope, executionValue)
	if err != nil {
		return execution.Request{}, err
	}
	return created.Execution.Request(), nil
}

var _ ExecutionService = (*PersistedExecutionService)(nil)
