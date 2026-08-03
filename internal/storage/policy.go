package storage

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/policies"
)

type CreatePolicyResult struct {
	Policy  policies.Policy
	Created bool
}

// PolicyRepository is a persistence contract only. Implementations must return
// policies in stable ID/version order and use optimistic lifecycle updates.
type PolicyRepository interface {
	FindPolicyByID(context.Context, Scope, string, uint64) (policies.Policy, error)
	FindApplicablePolicies(context.Context, Scope, policies.Applicability) ([]policies.Policy, error)
	CreatePolicy(context.Context, Scope, policies.Policy) (CreatePolicyResult, error)
	UpdatePolicy(ctx context.Context, scope Scope, policy policies.Policy, expectedRevision uint64) (policies.Policy, error)
}
