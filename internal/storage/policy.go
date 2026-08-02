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
	FindPolicyByID(context.Context, string, uint64) (policies.Policy, error)
	FindApplicablePolicies(context.Context, policies.Applicability) ([]policies.Policy, error)
	CreatePolicy(context.Context, policies.Policy) (CreatePolicyResult, error)
	UpdatePolicy(ctx context.Context, policy policies.Policy, expectedStatus policies.Status) (policies.Policy, error)
}
