package tools

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/services"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func swapDefinitions(service services.SwapService) ([]Definition, error) {
	preview, err := newDefinition(SwapPreviewName, "Validate a canonical Arc Mainnet USDC/EURC Swap without persisting or submitting it.", annotation(true), swapPreviewHandler(service))
	if err != nil {
		return nil, err
	}
	create, err := newDefinition(SwapCreateIntentName, "Create an immutable canonical SWAP intent; this does not approve or submit it.", annotation(false), swapCreateHandler(service))
	if err != nil {
		return nil, err
	}
	execute, err := newDefinition(SwapExecuteName, "Prepare execution from an already approved SWAP intent; frozen economics cannot be overridden.", annotation(false), swapExecuteHandler(service))
	if err != nil {
		return nil, err
	}
	status, err := newDefinition(SwapStatusName, "Read persisted SWAP execution state.", annotation(true), swapStatusHandler(service))
	if err != nil {
		return nil, err
	}
	return []Definition{preview, create, execute, status}, nil
}

func swapDraft(in SwapDraftInput) services.SwapDraft {
	return services.SwapDraft{ClientRequestID: in.ClientRequestID, Nonce: in.Nonce, WalletBindingID: in.WalletBindingID, TokenIn: in.TokenIn, TokenOut: in.TokenOut, AmountIn: in.AmountIn, ExpectedOutput: in.ExpectedOutput, MinAmountOut: in.MinAmountOut, MaxSlippageBPS: in.MaxSlippageBPS, MinHopPriceX36: in.MinHopPriceX36, QuoteID: in.QuoteID, QuoteSource: in.QuoteSource, EvidenceReference: in.EvidenceReference, QuoteExpiresAt: in.QuoteExpiresAt, SwapDeadline: in.SwapDeadline, Deadline: in.Deadline, PolicyReference: in.PolicyReference}
}

func swapPreviewHandler(service services.SwapService) sdkmcp.ToolHandlerFor[SwapDraftInput, SwapPreviewResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in SwapDraftInput) (*sdkmcp.CallToolResult, SwapPreviewResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), SwapPreviewResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		value, err := service.PreviewSwap(ctx, swapDraft(in))
		if err != nil {
			return errorResult(), SwapPreviewResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		out := SwapPreviewOutput{TokenIn: value.TokenIn.Symbol, TokenInAddress: value.TokenIn.Address, TokenOut: value.TokenOut.Symbol, TokenOutAddress: value.TokenOut.Address, AmountIn: value.AmountIn, ExpectedOutput: value.ExpectedOutput, MinAmountOut: value.MinAmountOut, MinHopPriceX36: value.MinHopPriceX36, SwapDeadline: value.SwapDeadline.UTC().Format("2006-01-02T15:04:05Z07:00"), QuoteExpiresAt: value.QuoteExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"), Executor: value.Executor, ChainID: value.ChainID, Network: value.Network}
		return nil, SwapPreviewResponse{Result: &out}, nil
	}
}
func swapCreateHandler(service services.SwapService) sdkmcp.ToolHandlerFor[SwapDraftInput, SwapIntentResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in SwapDraftInput) (*sdkmcp.CallToolResult, SwapIntentResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), SwapIntentResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		value, err := service.CreateSwapIntent(ctx, swapDraft(in))
		if err != nil {
			return errorResult(), SwapIntentResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		out := intentOutput(value)
		return nil, SwapIntentResponse{Result: &out}, nil
	}
}
func swapExecuteHandler(service services.SwapService) sdkmcp.ToolHandlerFor[SwapExecuteInput, SwapExecuteResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in SwapExecuteInput) (*sdkmcp.CallToolResult, SwapExecuteResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), SwapExecuteResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		value, err := service.ExecuteSwap(ctx, in.IntentID, in.ApprovalID, in.PolicyID, in.PolicyVersion)
		if err != nil {
			return errorResult(), SwapExecuteResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		out := executionOutput(value)
		return nil, SwapExecuteResponse{Result: &out}, nil
	}
}
func swapStatusHandler(service services.SwapService) sdkmcp.ToolHandlerFor[SwapStatusInput, SwapStatusResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in SwapStatusInput) (*sdkmcp.CallToolResult, SwapStatusResponse, error) {
		if err := in.Validate(); err != nil {
			return errorResult(), SwapStatusResponse{Error: validationToolError(in.RequestID, err)}, nil
		}
		value, err := service.SwapStatus(ctx, in.ExecutionID)
		if err != nil {
			return errorResult(), SwapStatusResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		out := executionOutput(value.Request())
		out.Status = string(value.Status())
		return nil, SwapStatusResponse{Result: &out}, nil
	}
}
