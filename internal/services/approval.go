package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/audit"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

// PersistedApprovalService is the authenticated application boundary for
// explicit approval artifacts. It never changes an intent or performs
// provider, signing, or execution work.
type PersistedApprovalService struct {
	Approvals  storage.ApprovalRepository
	Intents    storage.IntentRepository
	Authorizer auth.Authorizer
	Audit      storage.AuditRepository
	Now        func() time.Time
}

func (s *PersistedApprovalService) scope(ctx context.Context, permission auth.Permission) (storage.Scope, error) {
	if s == nil || s.Authorizer == nil {
		return storage.Scope{}, fmt.Errorf("approval service is not configured")
	}
	request, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		return storage.Scope{}, err
	}
	if err := s.Authorizer.Authorize(ctx, auth.AuthorizationInput{Request: request, Permission: permission}); err != nil {
		return storage.Scope{}, err
	}
	return requestauth.StorageScopeFromContext(ctx)
}

// RequestApproval creates or reuses the approval artifact for an immutable,
// frozen intent. The approval identity is deterministic from the intent
// identity and digest, so retries cannot create a different artifact.
func (s *PersistedApprovalService) RequestApproval(ctx context.Context, intentID string) (approvals.Approval, error) {
	scope, err := s.scope(ctx, auth.PermissionRequestApproval)
	if err != nil {
		return approvals.Approval{}, err
	}
	if s.Approvals == nil || s.Intents == nil || s.Now == nil {
		return approvals.Approval{}, fmt.Errorf("approval service is not configured")
	}

	intent, err := s.Intents.FindIntentByID(ctx, scope, intentID)
	if err != nil {
		return approvals.Approval{}, err
	}
	if intent.Ownership().UserID != scope.ActorID() {
		return approvals.Approval{}, fmt.Errorf("intent is not accessible")
	}
	if intent.Status() != intents.StatusApprovalRequired {
		return approvals.Approval{}, fmt.Errorf("intent is not awaiting approval")
	}

	// The repository performs the same lookup atomically with its create path;
	// this lookup avoids constructing a new artifact for the common replay case.
	if existing, findErr := s.Approvals.FindApprovalByIntent(ctx, scope, intent.IntentID(), intent.Version(), intent.Digest()); findErr == nil {
		return existing, nil
	}

	now := s.Now().UTC()
	approvalID, requestID := approvalIDs(intent)
	approval, err := approvals.New(approvals.Params{
		ApprovalID:        approvalID,
		Version:           1,
		ApprovalRequestID: requestID,
		CreatedAt:         now,
		ExpiresAt:         intent.ExpiresAt(),
	}, intent)
	if err != nil {
		return approvals.Approval{}, err
	}
	result, err := s.Approvals.CreateApproval(ctx, scope, approval)
	if err != nil {
		return approvals.Approval{}, err
	}
	if result.Created && s.Audit != nil {
		if err := s.Audit.AppendAudit(ctx, scope, audit.Record{
			Event: audit.Event{
				EventID:       scope.RequestID() + "/" + result.Approval.ApprovalID() + "/" + string(audit.EventApprovalRequested),
				Type:          audit.EventApprovalRequested,
				OccurredAt:    now,
				IntentID:      result.Approval.IntentID(),
				IntentVersion: result.Approval.IntentVersion(),
				IntentDigest:  result.Approval.IntentDigest(),
				ApprovalID:    result.Approval.ApprovalID(),
				UserID:        result.Approval.UserID(),
			},
			ActorType:       "user",
			ActorID:         scope.ActorID(),
			RequestID:       scope.RequestID(),
			TraceID:         scope.TraceID(),
			ResourceType:    "approval",
			ResourceID:      result.Approval.ApprovalID(),
			NewState:        string(result.Approval.Status()),
			SourceComponent: "approval.service",
		}); err != nil {
			return approvals.Approval{}, err
		}
	}
	return result.Approval, nil
}

func approvalIDs(intent intents.Intent) (string, string) {
	sum := sha256.Sum256([]byte("approval/" + intent.IntentID() + "/" + fmt.Sprint(intent.Version()) + "/" + intent.Digest()))
	return "apr_" + hex.EncodeToString(sum[:12]), "apr_req_" + hex.EncodeToString(sum[12:])
}

// GetApproval returns the authorized persisted approval artifact.
func (s *PersistedApprovalService) GetApproval(ctx context.Context, approvalID string) (approvals.Approval, error) {
	scope, err := s.scope(ctx, auth.PermissionReadApproval)
	if err != nil {
		return approvals.Approval{}, err
	}
	if s.Approvals == nil {
		return approvals.Approval{}, fmt.Errorf("approval service is not configured")
	}
	return s.Approvals.FindApprovalByID(ctx, scope, approvalID)
}

var _ ApprovalService = (*PersistedApprovalService)(nil)
