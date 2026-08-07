package services

import (
	"context"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

// PersistedPolicyService evaluates a persisted policy against a persisted
// intent and stores the resulting immutable decision reference.
type PersistedPolicyService struct {
	Intents     storage.IntentRepository
	Policies    storage.PolicyRepository
	Evaluations storage.PolicyEvaluationRepository
	Wallets     storage.WalletBindingRepository
	Authorizer  auth.Authorizer
	Now         func() time.Time
}

func (s *PersistedPolicyService) EvaluatePolicy(ctx context.Context, intentID, policyID string, policyVersion uint64, stage policies.EvaluationStage) (policies.Result, error) {
	if s == nil || s.Authorizer == nil || s.Intents == nil || s.Policies == nil || s.Evaluations == nil || s.Wallets == nil || s.Now == nil {
		return policies.Result{}, fmt.Errorf("policy service is not configured")
	}
	request, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		return policies.Result{}, err
	}
	if err := s.Authorizer.Authorize(ctx, auth.AuthorizationInput{Request: request, Permission: auth.PermissionEvaluatePolicy}); err != nil {
		return policies.Result{}, err
	}
	scope, err := requestauth.StorageScopeFromContext(ctx)
	if err != nil {
		return policies.Result{}, err
	}
	intent, err := s.Intents.FindIntentByID(ctx, scope, intentID)
	if err != nil {
		return policies.Result{}, err
	}
	policy, err := s.Policies.FindPolicyByID(ctx, scope, policyID, policyVersion)
	if err != nil {
		return policies.Result{}, err
	}
	binding, err := s.Wallets.FindBindingByID(ctx, scope, intent.Ownership().WalletBindingID)
	if err != nil {
		return policies.Result{}, err
	}
	identity, err := auth.NewIdentityContext(request.Identity(), request.Metadata())
	if err != nil {
		return policies.Result{}, err
	}
	if stage != policies.EvaluationStageBeforeApproval && stage != policies.EvaluationStageBeforeExecution {
		return policies.Result{}, fmt.Errorf("invalid policy evaluation stage")
	}
	at := s.Now().UTC()
	var result policies.Result
	if stage == policies.EvaluationStageBeforeApproval {
		result, err = policies.EvaluateForApproval(policy, intent, identity, binding, at)
	} else {
		result, err = policies.Evaluate(policy, intent, identity, binding, at)
	}
	if err != nil {
		return policies.Result{}, err
	}
	return s.Evaluations.CreatePolicyEvaluation(ctx, scope, result)
}

var _ PolicyService = (*PersistedPolicyService)(nil)
