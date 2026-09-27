package tools

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/services"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func sendDefinitions(service services.SendService) ([]Definition, error) {
	preview, err := newDefinition(SendPreviewName, "Validate an Arc Mainnet canonical ERC-20 Send without persisting or submitting it.", annotation(true), sendPreviewHandler(service))
	if err != nil {
		return nil, err
	}
	create, err := newDefinition(SendCreateIntentName, "Create an immutable typed SEND intent; this does not approve or submit it.", annotation(false), sendCreateHandler(service))
	if err != nil {
		return nil, err
	}
	execute, err := newDefinition(SendExecuteName, "Prepare execution from an already approved SEND intent; frozen financial fields cannot be overridden.", annotation(false), sendExecuteHandler(service))
	if err != nil {
		return nil, err
	}
	status, err := newDefinition(SendStatusName, "Read persisted SEND execution state.", annotation(true), sendStatusHandler(service))
	if err != nil {
		return nil, err
	}
	return []Definition{preview, create, execute, status}, nil
}

func sendDraft(in SendDraftInput) services.SendDraft {
	return services.SendDraft{ClientRequestID: in.ClientRequestID, Nonce: in.Nonce, WalletBindingID: in.WalletBindingID, TokenSymbol: in.Token, Recipient: in.Recipient, Amount: in.Amount, Deadline: in.Deadline, PolicyReference: in.PolicyReference}
}

func sendPreviewHandler(service services.SendService) sdkmcp.ToolHandlerFor[SendDraftInput, SendPreviewResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in SendDraftInput) (*sdkmcp.CallToolResult, SendPreviewResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), SendPreviewResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		value, err := service.PreviewSend(ctx, sendDraft(in))
		if err != nil {
			return errorResult(), SendPreviewResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		out := SendPreviewOutput{Token: value.Token.Symbol, TokenAddress: value.Token.Address, Recipient: value.Recipient, Amount: value.Amount, ChainID: value.ChainID, Network: value.Network}
		return nil, SendPreviewResponse{Result: &out}, nil
	}
}

func sendCreateHandler(service services.SendService) sdkmcp.ToolHandlerFor[SendDraftInput, SendIntentResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in SendDraftInput) (*sdkmcp.CallToolResult, SendIntentResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), SendIntentResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		value, err := service.CreateSendIntent(ctx, sendDraft(in))
		if err != nil {
			return errorResult(), SendIntentResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		out := intentOutput(value)
		return nil, SendIntentResponse{Result: &out}, nil
	}
}

func sendExecuteHandler(service services.SendService) sdkmcp.ToolHandlerFor[SendExecuteInput, SendExecuteResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in SendExecuteInput) (*sdkmcp.CallToolResult, SendExecuteResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), SendExecuteResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		value, err := service.ExecuteSend(ctx, in.IntentID, in.ApprovalID, in.PolicyID, in.PolicyVersion)
		if err != nil {
			return errorResult(), SendExecuteResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		out := executionOutput(value)
		return nil, SendExecuteResponse{Result: &out}, nil
	}
}

func sendStatusHandler(service services.SendService) sdkmcp.ToolHandlerFor[SendStatusInput, SendStatusResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in SendStatusInput) (*sdkmcp.CallToolResult, SendStatusResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), SendStatusResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		value, err := service.SendStatus(ctx, in.ExecutionID)
		if err != nil {
			return errorResult(), SendStatusResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		request := value.Request()
		out := executionOutput(request)
		out.Status = string(value.Status())
		return nil, SendStatusResponse{Result: &out}, nil
	}
}
