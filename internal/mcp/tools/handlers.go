package tools

import (
	"context"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/services"
)

func foundationDefinitions(bundle services.Bundle) ([]Definition, error) {
	createIntent, err := newDefinition(CreateIntentName, "Create a typed, provider-neutral financial intent; this does not approve or execute it.", annotation(false), createIntentHandler(bundle.Intents))
	if err != nil {
		return nil, err
	}
	getIntent, err := newDefinition(GetIntentName, "Read the safe lifecycle metadata for one authorized intent.", annotation(true), getIntentHandler(bundle.Intents))
	if err != nil {
		return nil, err
	}
	requestApproval, err := newDefinition(RequestApprovalName, "Create an explicit approval request for a frozen intent; this does not approve or execute it.", annotation(false), requestApprovalHandler(bundle.Approvals))
	if err != nil {
		return nil, err
	}
	getApproval, err := newDefinition(GetApprovalName, "Read the safe lifecycle metadata for one authorized approval request.", annotation(true), getApprovalHandler(bundle.Approvals))
	if err != nil {
		return nil, err
	}
	evaluatePolicy, err := newDefinition(EvaluatePolicyName, "Evaluate a referenced policy against an authorized intent; this does not execute the intent.", annotation(true), evaluatePolicyHandler(bundle.Policies))
	if err != nil {
		return nil, err
	}
	prepareExecution, err := newDefinition(PrepareExecutionName, "Prepare an immutable execution request from existing authorization references; this never submits or executes it.", annotation(false), prepareExecutionHandler(bundle.Executions))
	if err != nil {
		return nil, err
	}
	return []Definition{createIntent, getIntent, requestApproval, getApproval, evaluatePolicy, prepareExecution}, nil
}

func createIntentHandler(service services.IntentService) sdkmcp.ToolHandlerFor[CreateIntentInput, CreateIntentResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in CreateIntentInput) (*sdkmcp.CallToolResult, CreateIntentResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), CreateIntentResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		intent, err := service.CreateIntent(ctx, services.CreateIntentCommand{ClientRequestID: in.ClientRequestID, Nonce: in.Nonce, WalletBindingID: in.WalletBindingID, Type: in.IntentType, Financial: in.Financial, Route: in.Route, Deadline: in.Deadline, PolicyReference: in.PolicyReference})
		if err != nil {
			return errorResult(), CreateIntentResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		output := intentOutput(intent)
		return nil, CreateIntentResponse{Result: &output}, nil
	}
}
func getIntentHandler(service services.IntentService) sdkmcp.ToolHandlerFor[GetIntentInput, GetIntentResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in GetIntentInput) (*sdkmcp.CallToolResult, GetIntentResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), GetIntentResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		intent, err := service.GetIntent(ctx, in.IntentID)
		if err != nil {
			return errorResult(), GetIntentResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		output := intentOutput(intent)
		return nil, GetIntentResponse{Result: &output}, nil
	}
}
func requestApprovalHandler(service services.ApprovalService) sdkmcp.ToolHandlerFor[RequestApprovalInput, RequestApprovalResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in RequestApprovalInput) (*sdkmcp.CallToolResult, RequestApprovalResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), RequestApprovalResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		approval, err := service.RequestApproval(ctx, in.IntentID)
		if err != nil {
			return errorResult(), RequestApprovalResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		output := approvalOutput(approval)
		return nil, RequestApprovalResponse{Result: &output}, nil
	}
}
func getApprovalHandler(service services.ApprovalService) sdkmcp.ToolHandlerFor[GetApprovalInput, GetApprovalResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in GetApprovalInput) (*sdkmcp.CallToolResult, GetApprovalResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), GetApprovalResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		approval, err := service.GetApproval(ctx, in.ApprovalID)
		if err != nil {
			return errorResult(), GetApprovalResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		output := approvalOutput(approval)
		return nil, GetApprovalResponse{Result: &output}, nil
	}
}
func evaluatePolicyHandler(service services.PolicyService) sdkmcp.ToolHandlerFor[EvaluatePolicyInput, EvaluatePolicyResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in EvaluatePolicyInput) (*sdkmcp.CallToolResult, EvaluatePolicyResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), EvaluatePolicyResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		result, err := service.EvaluatePolicy(ctx, in.IntentID, in.PolicyID, in.PolicyVersion, policies.EvaluationStage(in.Stage))
		if err != nil {
			return errorResult(), EvaluatePolicyResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		output := policyOutput(result)
		return nil, EvaluatePolicyResponse{Result: &output}, nil
	}
}
func prepareExecutionHandler(service services.ExecutionService) sdkmcp.ToolHandlerFor[PrepareExecutionInput, PrepareExecutionResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in PrepareExecutionInput) (*sdkmcp.CallToolResult, PrepareExecutionResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), PrepareExecutionResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		request, err := service.PrepareExecution(ctx, in.IntentID, in.ApprovalID, in.PolicyID, in.PolicyVersion)
		if err != nil {
			return errorResult(), PrepareExecutionResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		output := executionOutput(request)
		return nil, PrepareExecutionResponse{Result: &output}, nil
	}
}

func intentOutput(intent intents.Intent) IntentOutput {
	owner := intent.Ownership()
	return IntentOutput{IntentID: intent.IntentID(), IntentVersion: intent.Version(), IntentDigest: intent.Digest(), IntentType: string(intent.Type()), Status: string(intent.Status()), WalletBindingID: owner.WalletBindingID, CreatedAt: formatTime(intent.CreatedAt()), ExpiresAt: formatTime(intent.ExpiresAt())}
}
func approvalOutput(approval approvals.Approval) ApprovalOutput {
	return ApprovalOutput{ApprovalID: approval.ApprovalID(), ApprovalVersion: approval.Version(), IntentID: approval.IntentID(), IntentVersion: approval.IntentVersion(), Status: string(approval.Status()), Decision: string(approval.Decision()), CreatedAt: formatTime(approval.CreatedAt()), ExpiresAt: formatTime(approval.ExpiresAt())}
}
func policyOutput(result policies.Result) PolicyOutput {
	findings := make([]PolicyFindingOutput, len(result.Findings))
	for i, finding := range result.Findings {
		findings[i] = PolicyFindingOutput{RuleID: finding.RuleID, RuleType: string(finding.RuleType), Decision: string(finding.Decision), Reason: string(finding.Reason)}
	}
	return PolicyOutput{PolicyID: result.PolicyID, PolicyVersion: result.PolicyVersion, IntentID: result.IntentID, IntentVersion: result.IntentVersion, IntentDigest: result.IntentDigest, Stage: string(result.Stage), Decision: string(result.Decision), EvaluatedAt: formatTime(result.EvaluatedAt), Findings: findings}
}
func executionOutput(request execution.Request) ExecutionOutput {
	return ExecutionOutput{ExecutionRequestID: request.RequestID(), ExecutionRequestKey: request.RequestKey(), ExecutionRequestVersion: request.Version(), ExecutionID: request.ExecutionID(), IntentID: request.IntentID(), ApprovalID: request.ApprovalID(), PolicyEvaluationKey: request.PolicyEvaluationKey(), Status: string(execution.StatusCreated), CreatedAt: formatTime(request.CreatedAt())}
}
func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
