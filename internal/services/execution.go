package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/audit"
	"github.com/deseti/wizpay-mcp/internal/auth"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
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
	Atomic      storage.AtomicRepository
	Wallets     storage.WalletBindingRepository
	Authorizer  auth.Authorizer
	Now         func() time.Time
}

func (s *PersistedExecutionService) PrepareExecution(ctx context.Context, intentID, approvalID, policyID string, policyVersion uint64) (execution.Request, error) {
	if s == nil || s.Authorizer == nil || s.Intents == nil || s.Approvals == nil || s.Policies == nil || s.Evaluations == nil || s.Executions == nil || s.Atomic == nil || s.Wallets == nil || s.Now == nil {
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
	if approval.Status() != approvals.StatusReadyForExecutionConfirmation && approval.Status() != approvals.StatusConsumed {
		return execution.Request{}, apperrors.New(apperrors.CodeExecutionNotAuthorized, "Wallet execution confirmation is required.", false, true, true)
	}
	operation, err := intents.NewOperationIdentity(intent)
	if err != nil {
		return execution.Request{}, err
	}
	if approval.Status() == approvals.StatusConsumed {
		if _, err := approval.Consume(approval.ConsumedAt(), operation); err != nil {
			return execution.Request{}, err
		}
		return s.replayConsumedExecution(ctx, scope, intent, approval, policyID, policyVersion, operation)
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
	consumed, err := approval.Consume(s.Now().UTC(), operation)
	if err != nil {
		return execution.Request{}, err
	}
	requestValue, err := execution.NewRequest(intent, consumed, result, s.Now().UTC())
	if err != nil {
		return execution.Request{}, err
	}
	executionValue, err := execution.New(requestValue)
	if err != nil {
		return execution.Request{}, err
	}
	record := audit.Record{Event: audit.Event{EventID: requestValue.RequestID() + "/execution-prepared", Type: audit.EventExecutionAuthorized, OccurredAt: s.Now().UTC(), IntentID: requestValue.IntentID(), IntentVersion: requestValue.IntentVersion(), IntentDigest: requestValue.IntentDigest(), ApprovalID: requestValue.ApprovalID(), ExecutionID: requestValue.ExecutionID(), ExecutionRequestID: requestValue.RequestID(), ExecutionRequestKey: requestValue.RequestKey(), ExecutionStatus: string(executionValue.Status()), UserID: scope.ActorID(), OperationKey: requestValue.OperationKey(), OperationVersion: requestValue.OperationVersion()}, ActorType: "user", ActorID: scope.ActorID(), RequestID: scope.RequestID(), TraceID: scope.TraceID(), ResourceType: "execution", ResourceID: requestValue.ExecutionID(), NewState: string(executionValue.Status()), SourceComponent: "execution.service"}
	_, created, err := s.Atomic.ConsumeApprovalAndCreateExecution(ctx, scope, consumed, approval.LifecycleRevision(), executionValue, record)
	if err != nil {
		return execution.Request{}, err
	}
	return created.Execution.Request(), nil
}

func (s *PersistedExecutionService) replayConsumedExecution(ctx context.Context, scope storage.Scope, intent intents.Intent, approval approvals.Approval, policyID string, policyVersion uint64, operation intents.OperationIdentity) (execution.Request, error) {
	existing, err := s.Executions.FindExecutionByOperationKey(ctx, scope, operation.OperationKey(), operation.Version())
	if err != nil {
		var appErr *apperrors.Error
		if errors.As(err, &appErr) && appErr.Code == apperrors.CodeExecutionNotFound {
			return execution.Request{}, apperrors.New(apperrors.CodeExecutionRecoverable, "Consumed approval has no durable execution and requires recovery.", false, true, true)
		}
		return execution.Request{}, err
	}
	request := existing.Request()
	owner := intent.Ownership()
	if approval.IntentID() != intent.IntentID() || approval.IntentVersion() != intent.Version() || approval.IntentDigest() != intent.Digest() ||
		approval.UserID() != owner.UserID || approval.WalletBindingID() != owner.WalletBindingID ||
		approval.WalletBindingVersion() != owner.WalletBindingVersion || approval.WalletID() != owner.WalletID ||
		approval.WalletAddress() != owner.WalletAddress || approval.ChainID() != owner.ChainID ||
		request.OperationKey() != operation.OperationKey() || request.OperationVersion() != operation.Version() ||
		request.IntentID() != intent.IntentID() || request.IntentVersion() != intent.Version() || request.IntentDigest() != intent.Digest() ||
		request.ApprovalID() != approval.ApprovalID() || request.ApprovalVersion() != approval.Version() ||
		request.PolicyID() != policyID || request.PolicyVersion() != policyVersion ||
		intent.Constraints().PolicyReference != fmt.Sprintf("%s:%d", request.PolicyID(), request.PolicyVersion()) {
		return execution.Request{}, apperrors.New(apperrors.CodeExecutionConflict, "Existing execution request does not match the consumed authorization.", false, true, true)
	}
	return request, nil
}

var _ ExecutionService = (*PersistedExecutionService)(nil)
