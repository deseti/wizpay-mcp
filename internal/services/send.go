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

type SendDraft struct {
	ClientRequestID string
	Nonce           string
	WalletBindingID string
	TokenSymbol     string
	Recipient       string
	Amount          intents.Amount
	Deadline        time.Time
	PolicyReference string
}

type SendPreview struct {
	Token     intents.Token
	Recipient string
	Amount    intents.Amount
	ChainID   string
	Network   string
}

type SendService interface {
	PreviewSend(context.Context, SendDraft) (SendPreview, error)
	CreateSendIntent(context.Context, SendDraft) (intents.Intent, error)
	ExecuteSend(context.Context, string, string, string, uint64) (execution.Request, error)
	SendStatus(context.Context, string) (execution.Execution, error)
}

// SendExecutionAuthority is the explicit seam for a future reviewed Arc
// Mainnet user/delegated signing provider and capability authorization.
// Track B supplies no implementation; nil means submission is unavailable.
type SendExecutionAuthority interface {
	AuthorizeSend(context.Context, intents.Intent) error
}

type PersistedSendService struct {
	Intents     *PersistedIntentService
	Executions  *PersistedExecutionService
	ExecutionDB storage.ExecutionRepository
	Wallets     storage.WalletBindingRepository
	Authorizer  auth.Authorizer
	Authority   SendExecutionAuthority
}

func (s *PersistedSendService) PreviewSend(ctx context.Context, draft SendDraft) (SendPreview, error) {
	if s == nil || s.Wallets == nil || s.Authorizer == nil {
		return SendPreview{}, fmt.Errorf("send service is not configured")
	}
	request, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		return SendPreview{}, err
	}
	if err := s.Authorizer.Authorize(ctx, auth.AuthorizationInput{Request: request, Permission: auth.PermissionCreateIntent}); err != nil {
		return SendPreview{}, err
	}
	scope, err := requestauth.StorageScopeFromContext(ctx)
	if err != nil {
		return SendPreview{}, err
	}
	binding, err := s.Wallets.FindBindingByID(ctx, scope, draft.WalletBindingID)
	if err != nil {
		return SendPreview{}, err
	}
	if err := binding.EnsureAuthorizable(scope.ActorID()); err != nil {
		return SendPreview{}, err
	}
	resource, err := contracts.CanonicalToken(draft.TokenSymbol)
	if err != nil {
		return SendPreview{}, apperrors.Wrap(apperrors.CodeValidationError, "Send is invalid.", false, true, true, err)
	}
	params := intents.SendParameters{Token: tokenFromResource(resource), Recipient: draft.Recipient, Amount: draft.Amount}
	// NewDraft is the single semantic validator, including self-send and exact
	// chain/network binding. Preview intentionally does not persist.
	now := time.Now().UTC()
	if s.Intents != nil && s.Intents.Now != nil {
		now = s.Intents.Now().UTC()
	}
	_, err = intents.NewDraft(intents.Params{IntentID: "preview", Version: 1, ClientRequestID: draft.ClientRequestID, Nonce: draft.Nonce, Type: intents.TypeSend,
		Ownership: intents.Ownership{UserID: scope.ActorID(), IdentityProvider: request.Principal().IdentityProvider(), WalletProvider: binding.Provider(), ProviderUserReference: binding.ProviderUserReference(), WalletBindingID: binding.BindingID(), WalletBindingVersion: binding.Version(), WalletID: binding.WalletID(), WalletAddress: binding.Address(), ChainID: binding.ChainID(), Network: binding.Network()},
		Financial: intents.FinancialParameters{Send: &params}, Route: intents.Route{Type: intents.RouteDirectWallet, Reference: intents.RouteReferenceSend, Version: intents.RouteVersionSend},
		Constraints: intents.Constraints{Deadline: draft.Deadline, PolicyReference: draft.PolicyReference}, CreatedAt: now, ExpiresAt: draft.Deadline})
	if err != nil {
		return SendPreview{}, err
	}
	return SendPreview{Token: params.Token, Recipient: contracts.NormalizeAddress(params.Recipient), Amount: params.Amount, ChainID: resource.ChainID, Network: resource.Network}, nil
}

func (s *PersistedSendService) CreateSendIntent(ctx context.Context, draft SendDraft) (intents.Intent, error) {
	if s == nil || s.Intents == nil {
		return intents.Intent{}, fmt.Errorf("send service is not configured")
	}
	resource, err := contracts.CanonicalToken(draft.TokenSymbol)
	if err != nil {
		return intents.Intent{}, apperrors.Wrap(apperrors.CodeValidationError, "Send is invalid.", false, true, true, err)
	}
	return s.Intents.CreateIntent(ctx, CreateIntentCommand{ClientRequestID: draft.ClientRequestID, Nonce: draft.Nonce, WalletBindingID: draft.WalletBindingID,
		Type: intents.TypeSend, Financial: intents.FinancialParameters{Send: &intents.SendParameters{Token: tokenFromResource(resource), Recipient: draft.Recipient, Amount: draft.Amount}},
		Route: intents.Route{Type: intents.RouteDirectWallet, Reference: intents.RouteReferenceSend, Version: intents.RouteVersionSend}, Deadline: draft.Deadline, PolicyReference: draft.PolicyReference})
}

func (s *PersistedSendService) ExecuteSend(ctx context.Context, intentID, approvalID, policyID string, policyVersion uint64) (execution.Request, error) {
	if s == nil || s.Intents == nil || s.Executions == nil || s.Authority == nil {
		return execution.Request{}, apperrors.New(apperrors.CodeCapabilityUnavailable, "Arc Mainnet Send authorization provider is unavailable.", false, true, true)
	}
	intent, err := s.Intents.GetIntent(ctx, intentID)
	if err != nil {
		return execution.Request{}, err
	}
	if intent.Type() != intents.TypeSend {
		return execution.Request{}, apperrors.New(apperrors.CodeValidationError, "SEND intent is required.", false, true, true)
	}
	if err := s.Authority.AuthorizeSend(ctx, intent); err != nil {
		return execution.Request{}, err
	}
	return s.Executions.PrepareExecution(ctx, intentID, approvalID, policyID, policyVersion)
}

func (s *PersistedSendService) SendStatus(ctx context.Context, executionID string) (execution.Execution, error) {
	if s == nil || s.ExecutionDB == nil || s.Intents == nil || s.Intents.Intents == nil || s.Authorizer == nil {
		return execution.Execution{}, fmt.Errorf("send service is not configured")
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
	if intent.Type() != intents.TypeSend || intent.Digest() != value.Request().IntentDigest() {
		return execution.Execution{}, apperrors.New(apperrors.CodeValidationError, "Execution is not a SEND execution.", false, true, true)
	}
	return value, nil
}

func tokenFromResource(resource contracts.TokenResource) intents.Token {
	return intents.Token{ChainID: resource.ChainID, Standard: "ERC20", Address: resource.Address, Symbol: resource.Symbol, Decimals: resource.Decimals}
}

var _ SendService = (*PersistedSendService)(nil)
