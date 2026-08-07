package storage

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/approvals"
)

type CreateApprovalResult struct {
	Approval approvals.Approval
	Created  bool
}

type ApprovalListOptions struct {
	Limit  int
	Cursor string
	Status string
}

// ApprovalRepository stores explicit artifacts only. Implementations must
// preserve the exact intent digest and wallet-binding version and update state
// using optimistic lifecycle checks.
type ApprovalRepository interface {
	FindApprovalByID(context.Context, Scope, string) (approvals.Approval, error)
	FindApprovalByIntent(context.Context, Scope, string, uint64, string) (approvals.Approval, error)
	ListApprovals(context.Context, Scope, ApprovalListOptions) ([]approvals.Approval, error)
	CreateApproval(context.Context, Scope, approvals.Approval) (CreateApprovalResult, error)
	UpdateApproval(ctx context.Context, scope Scope, approval approvals.Approval, expectedRevision uint64) (approvals.Approval, error)
}
