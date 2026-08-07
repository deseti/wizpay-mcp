package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/autonomy"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

// StepUpCoordinator uses the existing immutable intent and approval
// repositories. It never creates a second intent or performs provider work.
type StepUpCoordinator struct {
	Intents   storage.IntentRepository
	Approvals storage.ApprovalRepository
	Autonomy  storage.AutonomyRepository
	Now       func() time.Time
}

// RequestForOccurrence creates or reuses the existing approval for the frozen
// intent and binds that approval to the same durable occurrence. It never
// creates a replacement intent or grants execution authority.
func (s *StepUpCoordinator) RequestForOccurrence(ctx context.Context, scope storage.Scope, occurrence autonomy.Occurrence) (approvals.Approval, error) {
	if s == nil || s.Autonomy == nil || occurrence.IntentID == "" {
		return approvals.Approval{}, fmt.Errorf("step-up occurrence binding is not configured")
	}
	approval, err := s.Request(ctx, scope, occurrence.IntentID)
	if err != nil {
		return approvals.Approval{}, err
	}
	if err := s.Autonomy.BindAutonomyApproval(ctx, scope, occurrence, approval.ApprovalID()); err != nil {
		return approvals.Approval{}, err
	}
	return approval, nil
}

func (s *StepUpCoordinator) Request(ctx context.Context, scope storage.Scope, intentID string) (approvals.Approval, error) {
	if s == nil || s.Intents == nil || s.Approvals == nil || s.Now == nil {
		return approvals.Approval{}, fmt.Errorf("step-up coordinator is not configured")
	}
	intent, err := s.Intents.FindIntentByID(ctx, scope, intentID)
	if err != nil {
		return approvals.Approval{}, err
	}
	now := s.Now().UTC()
	if intent.Status() == intents.StatusCreated {
		next, transitionErr := intent.Transition(intents.StatusApprovalRequired, now)
		if transitionErr != nil {
			return approvals.Approval{}, transitionErr
		}
		intent, err = s.Intents.UpdateIntent(ctx, scope, next, intent.LifecycleRevision())
		if err != nil {
			return approvals.Approval{}, err
		}
	}
	if intent.Status() != intents.StatusApprovalRequired {
		return approvals.Approval{}, fmt.Errorf("intent is not awaiting step-up approval")
	}
	existing, findErr := s.Approvals.FindApprovalByIntent(ctx, scope, intent.IntentID(), intent.Version(), intent.Digest())
	if findErr == nil {
		return existing, nil
	}
	sum := sha256.Sum256([]byte("step-up/" + intent.IntentID() + "/" + intent.Digest()))
	id := "apr_auto_" + hex.EncodeToString(sum[:12])
	requestID := "apr_req_auto_" + hex.EncodeToString(sum[12:])
	approval, err := approvals.New(approvals.Params{ApprovalID: id, Version: 1, ApprovalRequestID: requestID, CreatedAt: now, ExpiresAt: intent.ExpiresAt()}, intent)
	if err != nil {
		return approvals.Approval{}, err
	}
	result, err := s.Approvals.CreateApproval(ctx, scope, approval)
	if err != nil {
		return approvals.Approval{}, err
	}
	return result.Approval, nil
}
func (s *StepUpCoordinator) Valid(ctx context.Context, scope storage.Scope, intent intents.Intent, approvalID string) error {
	if approvalID == "" {
		return fmt.Errorf("step-up approval is required")
	}
	approval, err := s.Approvals.FindApprovalByID(ctx, scope, approvalID)
	if err != nil {
		return err
	}
	if approval.Status() != approvals.StatusApproved {
		return fmt.Errorf("step-up approval is not approved")
	}
	return approval.EnsureAuthorizes(intent, s.Now().UTC())
}
