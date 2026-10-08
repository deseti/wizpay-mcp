package services

import (
	"context"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/contracts"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

type SwapDraft struct {
	ClientRequestID, Nonce, WalletBindingID string
	TokenIn, TokenOut                       string
	AmountIn, ExpectedOutput, MinAmountOut  intents.Amount
	MaxSlippageBPS                          uint16
	MinHopPriceX36                          string
	QuoteID, QuoteSource, EvidenceReference string
	QuoteExpiresAt, SwapDeadline, Deadline  time.Time
	PolicyReference                         string
}

type SwapPreview struct {
	TokenIn, TokenOut                      intents.Token
	AmountIn, ExpectedOutput, MinAmountOut intents.Amount
	MinHopPriceX36                         string
	SwapDeadline, QuoteExpiresAt           time.Time
	ChainID, Network, Executor             string
}

type SwapService interface {
	PreviewSwap(context.Context, SwapDraft) (SwapPreview, error)
	CreateSwapIntent(context.Context, SwapDraft) (intents.Intent, error)
	ExecuteSwap(context.Context, string, string, string, uint64) (execution.Request, error)
	SwapStatus(context.Context, string) (execution.Execution, error)
}

type SwapExecutionAuthority interface {
	AuthorizeSwap(context.Context, intents.Intent) error
}

type PersistedSwapService struct {
	Intents     *PersistedIntentService
	Executions  *PersistedExecutionService
	ExecutionDB storage.ExecutionRepository
	Wallets     storage.WalletBindingRepository
	Authorizer  auth.Authorizer
	Authority   SwapExecutionAuthority
}

func swapFinancial(draft SwapDraft, wallet string) (intents.SwapParameters, error) {
	input, err := contracts.CanonicalToken(draft.TokenIn)
	if err != nil {
		return intents.SwapParameters{}, err
	}
	output, err := contracts.CanonicalToken(draft.TokenOut)
	if err != nil {
		return intents.SwapParameters{}, err
	}
	params := intents.SwapParameters{
		SchemaVersion: intents.FinancialSchemaPhase12,
		InputToken:    tokenFromResource(input), OutputToken: tokenFromResource(output),
		InputAmount: draft.AmountIn, ExpectedOutput: draft.ExpectedOutput, MinimumOutput: draft.MinAmountOut,
		QuoteReference: draft.QuoteID, MaxSlippageBPS: draft.MaxSlippageBPS, MinHopPriceX36: draft.MinHopPriceX36,
		Router: contracts.AddressUniswapUniversalRouter, Recipient: wallet,
		Quote:    &intents.SwapQuote{QuoteID: draft.QuoteID, Source: draft.QuoteSource, ExpectedAmountOut: draft.ExpectedOutput, MinAmountOut: draft.MinAmountOut, Router: contracts.AddressUniswapUniversalRouter, ExpiresAt: draft.QuoteExpiresAt, EvidenceReference: draft.EvidenceReference},
		Deadline: draft.SwapDeadline,
	}
	if !params.Phase12Executable() {
		return intents.SwapParameters{}, fmt.Errorf("swap economic material is invalid")
	}
	return params, nil
}

func (s *PersistedSwapService) resolveWallet(ctx context.Context, bindingID string) (auth.TrustedRequest, storage.Scope, string, error) {
	if s == nil || s.Wallets == nil || s.Authorizer == nil {
		return auth.TrustedRequest{}, storage.Scope{}, "", fmt.Errorf("swap service is not configured")
	}
	request, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		return auth.TrustedRequest{}, storage.Scope{}, "", err
	}
	if err := s.Authorizer.Authorize(ctx, auth.AuthorizationInput{Request: request, Permission: auth.PermissionCreateIntent}); err != nil {
		return auth.TrustedRequest{}, storage.Scope{}, "", err
	}
	scope, err := requestauth.StorageScopeFromContext(ctx)
	if err != nil {
		return auth.TrustedRequest{}, storage.Scope{}, "", err
	}
	binding, err := s.Wallets.FindBindingByID(ctx, scope, bindingID)
	if err != nil {
		return auth.TrustedRequest{}, storage.Scope{}, "", err
	}
	if err := binding.EnsureAuthorizable(scope.ActorID()); err != nil {
		return auth.TrustedRequest{}, storage.Scope{}, "", err
	}
	return request, scope, binding.Address(), nil
}

func (s *PersistedSwapService) PreviewSwap(ctx context.Context, draft SwapDraft) (SwapPreview, error) {
	request, scope, wallet, err := s.resolveWallet(ctx, draft.WalletBindingID)
	if err != nil {
		return SwapPreview{}, err
	}
	params, err := swapFinancial(draft, wallet)
	if err != nil {
		return SwapPreview{}, apperrors.Wrap(apperrors.CodeValidationError, "Swap is invalid.", false, true, true, err)
	}
	binding, err := s.Wallets.FindBindingByID(ctx, scope, draft.WalletBindingID)
	if err != nil {
		return SwapPreview{}, err
	}
	now := time.Now().UTC()
	if s.Intents != nil && s.Intents.Now != nil {
		now = s.Intents.Now().UTC()
	}
	_, err = intents.NewDraft(intents.Params{IntentID: "preview", Version: 1, ClientRequestID: draft.ClientRequestID, Nonce: draft.Nonce, Type: intents.TypeSwap,
		Ownership: intents.Ownership{UserID: scope.ActorID(), IdentityProvider: request.Principal().IdentityProvider(), WalletProvider: binding.Provider(), ProviderUserReference: binding.ProviderUserReference(), WalletBindingID: binding.BindingID(), WalletBindingVersion: binding.Version(), WalletID: binding.WalletID(), WalletAddress: binding.Address(), ChainID: binding.ChainID(), Network: binding.Network()},
		Financial: intents.FinancialParameters{Swap: &params}, Route: intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferenceSwap, Version: intents.RouteVersionSwap}, Constraints: intents.Constraints{Deadline: draft.Deadline, PolicyReference: draft.PolicyReference}, CreatedAt: now, ExpiresAt: draft.Deadline})
	if err != nil {
		return SwapPreview{}, err
	}
	return SwapPreview{TokenIn: params.InputToken, TokenOut: params.OutputToken, AmountIn: params.InputAmount, ExpectedOutput: params.ExpectedOutput, MinAmountOut: params.MinimumOutput, MinHopPriceX36: params.MinHopPriceX36, SwapDeadline: params.Deadline, QuoteExpiresAt: params.Quote.ExpiresAt, ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet, Executor: contracts.AddressWizPaySwapExecutor}, nil
}

func (s *PersistedSwapService) CreateSwapIntent(ctx context.Context, draft SwapDraft) (intents.Intent, error) {
	if s == nil || s.Intents == nil {
		return intents.Intent{}, fmt.Errorf("swap service is not configured")
	}
	_, _, wallet, err := s.resolveWallet(ctx, draft.WalletBindingID)
	if err != nil {
		return intents.Intent{}, err
	}
	params, err := swapFinancial(draft, wallet)
	if err != nil {
		return intents.Intent{}, apperrors.Wrap(apperrors.CodeValidationError, "Swap is invalid.", false, true, true, err)
	}
	return s.Intents.CreateIntent(ctx, CreateIntentCommand{ClientRequestID: draft.ClientRequestID, Nonce: draft.Nonce, WalletBindingID: draft.WalletBindingID, Type: intents.TypeSwap, Financial: intents.FinancialParameters{Swap: &params}, Route: intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferenceSwap, Version: intents.RouteVersionSwap}, Deadline: draft.Deadline, PolicyReference: draft.PolicyReference})
}

func (s *PersistedSwapService) ExecuteSwap(ctx context.Context, intentID, approvalID, policyID string, policyVersion uint64) (execution.Request, error) {
	if s == nil || s.Intents == nil || s.Executions == nil || s.Authority == nil {
		return execution.Request{}, apperrors.New(apperrors.CodeCapabilityUnavailable, "Arc Mainnet Swap authorization provider is unavailable.", false, true, true)
	}
	intent, err := s.Intents.GetIntent(ctx, intentID)
	if err != nil {
		return execution.Request{}, err
	}
	if intent.Type() != intents.TypeSwap || intent.Financial().Swap == nil || !intent.Financial().Swap.Phase12Executable() {
		return execution.Request{}, apperrors.New(apperrors.CodeValidationError, "Canonical SWAP intent is required.", false, true, true)
	}
	if err := s.Authority.AuthorizeSwap(ctx, intent); err != nil {
		return execution.Request{}, err
	}
	return s.Executions.PrepareExecution(ctx, intentID, approvalID, policyID, policyVersion)
}

func (s *PersistedSwapService) SwapStatus(ctx context.Context, executionID string) (execution.Execution, error) {
	if s == nil || s.ExecutionDB == nil || s.Intents == nil || s.Intents.Intents == nil || s.Authorizer == nil {
		return execution.Execution{}, fmt.Errorf("swap service is not configured")
	}
	request, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		return execution.Execution{}, err
	}
	if err := s.Authorizer.Authorize(ctx, auth.AuthorizationInput{Request: request, Permission: auth.PermissionPrepareExecution}); err != nil {
		return execution.Execution{}, err
	}
	scope, err := requestauth.StorageScopeFromContext(ctx)
	if err != nil {
		return execution.Execution{}, err
	}
	value, err := s.ExecutionDB.FindExecutionByID(ctx, scope, executionID)
	if err != nil {
		return execution.Execution{}, err
	}
	intent, err := s.Intents.Intents.FindIntentByID(ctx, scope, value.Request().IntentID())
	if err != nil {
		return execution.Execution{}, err
	}
	if intent.Type() != intents.TypeSwap || intent.Digest() != value.Request().IntentDigest() {
		return execution.Execution{}, apperrors.New(apperrors.CodeValidationError, "Execution is not a SWAP execution.", false, true, true)
	}
	return value, nil
}

var _ SwapService = (*PersistedSwapService)(nil)
