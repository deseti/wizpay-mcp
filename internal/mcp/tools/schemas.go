package tools

import (
	"time"

	"github.com/deseti/wizpay-mcp/internal/intents"
)

type CreateIntentInput struct {
	RequestID       string                      `json:"request_id" jsonschema:"client correlation identifier"`
	ClientRequestID string                      `json:"client_request_id" jsonschema:"idempotency identifier chosen by the client"`
	Nonce           string                      `json:"nonce" jsonschema:"unique intent nonce"`
	WalletBindingID string                      `json:"wallet_binding_id" jsonschema:"previously authorized wallet binding reference"`
	IntentType      intents.Type                `json:"intent_type" jsonschema:"one of PAYROLL, SWAP, BRIDGE, ANS_REGISTRATION"`
	Financial       intents.FinancialParameters `json:"financial" jsonschema:"exactly one typed financial payload matching intent_type"`
	Route           intents.Route               `json:"route" jsonschema:"approved provider-neutral route reference"`
	Deadline        time.Time                   `json:"deadline" jsonschema:"authorization deadline in RFC3339 format"`
	PolicyReference string                      `json:"policy_reference" jsonschema:"immutable policy reference"`
}

type GetIntentInput struct {
	RequestID string `json:"request_id"`
	IntentID  string `json:"intent_id"`
}

type RequestApprovalInput struct {
	RequestID string `json:"request_id"`
	IntentID  string `json:"intent_id"`
}

type GetApprovalInput struct {
	RequestID  string `json:"request_id"`
	ApprovalID string `json:"approval_id"`
}

type EvaluatePolicyInput struct {
	RequestID     string `json:"request_id"`
	IntentID      string `json:"intent_id"`
	PolicyID      string `json:"policy_id"`
	PolicyVersion uint64 `json:"policy_version"`
	Stage         string `json:"stage" jsonschema:"one of BEFORE_APPROVAL, BEFORE_EXECUTION"`
}

type PrepareExecutionInput struct {
	RequestID     string `json:"request_id"`
	IntentID      string `json:"intent_id"`
	ApprovalID    string `json:"approval_id"`
	PolicyID      string `json:"policy_id"`
	PolicyVersion uint64 `json:"policy_version"`
}

type ToolError struct {
	Code               string `json:"code"`
	Message            string `json:"message"`
	RequestID          string `json:"request_id"`
	Retryable          bool   `json:"retryable"`
	UserActionRequired bool   `json:"user_action_required"`
	Terminal           bool   `json:"terminal"`
}

type IntentOutput struct {
	IntentID        string `json:"intent_id"`
	IntentVersion   uint64 `json:"intent_version"`
	IntentDigest    string `json:"intent_digest,omitempty"`
	IntentType      string `json:"intent_type"`
	Status          string `json:"status"`
	WalletBindingID string `json:"wallet_binding_id"`
	CreatedAt       string `json:"created_at"`
	ExpiresAt       string `json:"expires_at"`
}

type ApprovalOutput struct {
	ApprovalID      string `json:"approval_id"`
	ApprovalVersion uint64 `json:"approval_version"`
	IntentID        string `json:"intent_id"`
	IntentVersion   uint64 `json:"intent_version"`
	Status          string `json:"status"`
	Decision        string `json:"decision"`
	CreatedAt       string `json:"created_at"`
	ExpiresAt       string `json:"expires_at"`
}

type PolicyFindingOutput struct {
	RuleID   string `json:"rule_id"`
	RuleType string `json:"rule_type"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

type PolicyOutput struct {
	PolicyID      string                `json:"policy_id"`
	PolicyVersion uint64                `json:"policy_version"`
	IntentID      string                `json:"intent_id"`
	IntentVersion uint64                `json:"intent_version"`
	IntentDigest  string                `json:"intent_digest"`
	Stage         string                `json:"stage"`
	Decision      string                `json:"decision"`
	EvaluatedAt   string                `json:"evaluated_at"`
	Findings      []PolicyFindingOutput `json:"findings"`
}

type ExecutionOutput struct {
	ExecutionRequestID      string `json:"execution_request_id"`
	ExecutionRequestKey     string `json:"execution_request_key"`
	ExecutionRequestVersion uint64 `json:"execution_request_version"`
	ExecutionID             string `json:"execution_id"`
	IntentID                string `json:"intent_id"`
	ApprovalID              string `json:"approval_id"`
	PolicyEvaluationKey     string `json:"policy_evaluation_key"`
	Status                  string `json:"status"`
	CreatedAt               string `json:"created_at"`
}

type CreateIntentResponse struct {
	Result *IntentOutput `json:"result,omitempty"`
	Error  *ToolError    `json:"error,omitempty"`
}
type GetIntentResponse struct {
	Result *IntentOutput `json:"result,omitempty"`
	Error  *ToolError    `json:"error,omitempty"`
}
type RequestApprovalResponse struct {
	Result *ApprovalOutput `json:"result,omitempty"`
	Error  *ToolError      `json:"error,omitempty"`
}
type GetApprovalResponse struct {
	Result *ApprovalOutput `json:"result,omitempty"`
	Error  *ToolError      `json:"error,omitempty"`
}
type EvaluatePolicyResponse struct {
	Result *PolicyOutput `json:"result,omitempty"`
	Error  *ToolError    `json:"error,omitempty"`
}
type PrepareExecutionResponse struct {
	Result *ExecutionOutput `json:"result,omitempty"`
	Error  *ToolError       `json:"error,omitempty"`
}
