package tools

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/services"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func payrollDefinitions(service services.PayrollService) ([]Definition, error) {
	preview, err := newDefinition(PayrollPreviewName, "Validate an Arc Mainnet same-token or canonical cross-token Payroll without persisting or submitting it.", annotation(true), payrollPreviewHandler(service))
	if err != nil {
		return nil, err
	}
	create, err := newDefinition(PayrollCreateIntentName, "Create an immutable same-token or canonical cross-token PAYROLL intent; this does not approve or submit it.", annotation(false), payrollCreateHandler(service))
	if err != nil {
		return nil, err
	}
	execute, err := newDefinition(PayrollExecuteName, "Prepare execution from an already approved PAYROLL intent; frozen financial fields cannot be overridden.", annotation(false), payrollExecuteHandler(service))
	if err != nil {
		return nil, err
	}
	status, err := newDefinition(PayrollStatusName, "Read persisted PAYROLL execution state.", annotation(true), payrollStatusHandler(service))
	if err != nil {
		return nil, err
	}
	return []Definition{preview, create, execute, status}, nil
}
func payrollDraft(in PayrollDraftInput) services.PayrollDraft {
	recipients := make([]services.PayrollRecipientDraft, len(in.Recipients))
	for i, r := range in.Recipients {
		recipients[i] = services.PayrollRecipientDraft{Address: r.Address, Amount: r.Amount}
	}
	return services.PayrollDraft{ClientRequestID: in.ClientRequestID, Nonce: in.Nonce, WalletBindingID: in.WalletBindingID, TokenSymbol: in.Token, OutputTokenSymbol: in.OutputToken, Recipients: recipients, ReferenceID: in.ReferenceID, Deadline: in.Deadline, GrossInput: in.GrossInput, MinTotalOut: in.MinTotalOut, MinHopPriceX36: in.MinHopPriceX36, SwapDeadline: in.SwapDeadline, PolicyReference: in.PolicyReference}
}
func payrollPreviewHandler(service services.PayrollService) sdkmcp.ToolHandlerFor[PayrollDraftInput, PayrollPreviewResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in PayrollDraftInput) (*sdkmcp.CallToolResult, PayrollPreviewResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), PayrollPreviewResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		value, err := service.PreviewPayroll(ctx, payrollDraft(in))
		if err != nil {
			return errorResult(), PayrollPreviewResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		recipients := make([]PayrollRecipientInput, len(value.Recipients))
		for i, r := range value.Recipients {
			recipients[i] = PayrollRecipientInput{Address: r.Address, Amount: r.Amount}
		}
		out := PayrollPreviewOutput{Token: value.Token.Symbol, TokenAddress: value.Token.Address, OutputToken: value.OutputToken.Symbol, OutputTokenAddress: value.OutputToken.Address, Recipients: recipients, Total: value.Total, GrossInput: value.GrossInput, MinTotalOut: value.MinTotalOut, MinHopPriceX36: value.MinHopPriceX36, ReferenceID: value.ReferenceID, ChainID: value.ChainID, Network: value.Network}
		if !value.SwapDeadline.IsZero() {
			out.SwapDeadline = value.SwapDeadline.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		return nil, PayrollPreviewResponse{Result: &out}, nil
	}
}
func payrollCreateHandler(service services.PayrollService) sdkmcp.ToolHandlerFor[PayrollDraftInput, PayrollIntentResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in PayrollDraftInput) (*sdkmcp.CallToolResult, PayrollIntentResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), PayrollIntentResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		value, err := service.CreatePayrollIntent(ctx, payrollDraft(in))
		if err != nil {
			return errorResult(), PayrollIntentResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		out := intentOutput(value)
		return nil, PayrollIntentResponse{Result: &out}, nil
	}
}
func payrollExecuteHandler(service services.PayrollService) sdkmcp.ToolHandlerFor[PayrollExecuteInput, PayrollExecuteResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in PayrollExecuteInput) (*sdkmcp.CallToolResult, PayrollExecuteResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), PayrollExecuteResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		value, err := service.ExecutePayroll(ctx, in.IntentID, in.ApprovalID, in.PolicyID, in.PolicyVersion)
		if err != nil {
			return errorResult(), PayrollExecuteResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		out := executionOutput(value)
		return nil, PayrollExecuteResponse{Result: &out}, nil
	}
}
func payrollStatusHandler(service services.PayrollService) sdkmcp.ToolHandlerFor[PayrollStatusInput, PayrollStatusResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in PayrollStatusInput) (*sdkmcp.CallToolResult, PayrollStatusResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), PayrollStatusResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		value, err := service.PayrollStatus(ctx, in.ExecutionID)
		if err != nil {
			return errorResult(), PayrollStatusResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		out := executionOutput(value.Request())
		out.Status = string(value.Status())
		return nil, PayrollStatusResponse{Result: &out}, nil
	}
}
