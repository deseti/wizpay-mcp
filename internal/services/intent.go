package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/audit"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

// PersistedIntentService is the authenticated application boundary for
// creating and reading immutable intent values.
type PersistedIntentService struct {
	Intents    storage.IntentRepository
	Wallets    storage.WalletBindingRepository
	Authorizer auth.Authorizer
	Audit      storage.AuditRepository
	Now        func() time.Time
}

func (s *PersistedIntentService) scope(ctx context.Context, permission auth.Permission) (storage.Scope, error) {
	if s == nil || s.Authorizer == nil {
		return storage.Scope{}, fmt.Errorf("intent service is not configured")
	}
	request, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		return storage.Scope{}, err
	}
	if err := s.Authorizer.Authorize(ctx, auth.AuthorizationInput{Request: request, Permission: permission}); err != nil {
		return storage.Scope{}, err
	}
	return requestauth.StorageScopeFromContext(ctx)
}

func (s *PersistedIntentService) CreateIntent(ctx context.Context, command CreateIntentCommand) (intents.Intent, error) {
	scope, err := s.scope(ctx, auth.PermissionCreateIntent)
	if err != nil {
		return intents.Intent{}, err
	}
	if s.Intents == nil || s.Wallets == nil || s.Now == nil {
		return intents.Intent{}, fmt.Errorf("intent service is not configured")
	}
	request, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		return intents.Intent{}, err
	}
	binding, err := s.Wallets.FindBindingByID(ctx, scope, command.WalletBindingID)
	if err != nil {
		return intents.Intent{}, err
	}
	if err := binding.EnsureAuthorizable(scope.ActorID()); err != nil {
		return intents.Intent{}, err
	}
	owner := intents.Ownership{UserID: scope.ActorID(), IdentityProvider: request.Principal().IdentityProvider(), ProviderUserReference: binding.ProviderUserReference(), WalletBindingID: binding.BindingID(), WalletBindingVersion: binding.Version(), WalletID: binding.WalletID(), WalletAddress: binding.Address(), ChainID: binding.ChainID(), Network: binding.Network()}
	now := s.Now().UTC()
	params := intents.Params{IntentID: intentID(scope, command.ClientRequestID), Version: 1, ClientRequestID: command.ClientRequestID, Nonce: command.Nonce, Type: command.Type, Ownership: owner, Financial: command.Financial, Route: command.Route, Constraints: intents.Constraints{Deadline: command.Deadline, PolicyReference: command.PolicyReference}, CreatedAt: now, ExpiresAt: command.Deadline}
	value, err := intents.NewDraft(params)
	if err != nil {
		return intents.Intent{}, err
	}
	value, err = value.Transition(intents.StatusCreated, now)
	if err != nil {
		return intents.Intent{}, err
	}
	result, err := s.Intents.CreateIntent(ctx, scope, value)
	if err != nil {
		return intents.Intent{}, err
	}
	if result.Created && s.Audit != nil {
		if err := s.Audit.AppendAudit(ctx, scope, audit.Record{Event: audit.Event{EventID: scope.RequestID() + "/" + result.Intent.IntentID() + "/" + string(audit.EventIntentCreated), Type: audit.EventIntentCreated, OccurredAt: now, IntentID: result.Intent.IntentID(), IntentVersion: result.Intent.Version(), IntentDigest: result.Intent.Digest(), WalletBindingID: result.Intent.Ownership().WalletBindingID, WalletBindingVersion: result.Intent.Ownership().WalletBindingVersion, UserID: result.Intent.Ownership().UserID}, ActorType: "user", ActorID: scope.ActorID(), RequestID: scope.RequestID(), TraceID: scope.TraceID(), ResourceType: "intent", ResourceID: result.Intent.IntentID(), NewState: string(result.Intent.Status()), SourceComponent: "intent.service"}); err != nil {
			return intents.Intent{}, err
		}
	}
	return result.Intent, nil
}

func intentID(scope storage.Scope, clientRequestID string) string {
	sum := sha256.Sum256([]byte(scope.TenantID() + "/" + clientRequestID))
	return "int_" + hex.EncodeToString(sum[:12])
}

func (s *PersistedIntentService) GetIntent(ctx context.Context, id string) (intents.Intent, error) {
	scope, err := s.scope(ctx, auth.PermissionReadIntent)
	if err != nil {
		return intents.Intent{}, err
	}
	if s.Intents == nil {
		return intents.Intent{}, fmt.Errorf("intent service is not configured")
	}
	return s.Intents.FindIntentByID(ctx, scope, id)
}

var _ IntentService = (*PersistedIntentService)(nil)
