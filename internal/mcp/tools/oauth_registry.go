package tools

import (
	"fmt"

	"github.com/deseti/wizpay-mcp/internal/services"
)

// NewOAuthReadOnlyRegistry is the explicit capability allowlist for mcp:read.
// It reuses the authorized read handlers without registering mutation handlers.
func NewOAuthReadOnlyRegistry(intents services.IntentService, approvals services.ApprovalService) (*Registry, error) {
	if intents == nil || approvals == nil {
		return nil, fmt.Errorf("OAuth read services are required")
	}
	getIntent, err := newDefinition(GetIntentName, "Read the safe lifecycle metadata for one authorized intent.", annotation(true), getIntentHandler(intents))
	if err != nil {
		return nil, err
	}
	getApproval, err := newDefinition(GetApprovalName, "Read the safe lifecycle metadata for one authorized approval request.", annotation(true), getApprovalHandler(approvals))
	if err != nil {
		return nil, err
	}
	return NewRegistry(getIntent, getApproval)
}
