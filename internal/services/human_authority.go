package services

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

func (s *PersistedApprovalService) humanScope(ctx context.Context, permission auth.Permission) (storage.Scope, error) {
	if err := auth.RequireHuman(ctx, permission); err != nil {
		return storage.Scope{}, err
	}
	return s.scope(ctx, permission)
}
func (s *PersistedApprovalService) validateApprovalBinding(ctx context.Context, scope storage.Scope, a approvals.Approval, intent intents.Intent) error {
	o := intent.Ownership()
	request, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		return err
	}
	if o.IdentityProvider != request.Identity().Provider() {
		return apperrors.New(apperrors.CodeAuthorizationRequired, "Intent authentication identity does not match.", false, true, true)
	}
	if o.UserID != scope.ActorID() || a.UserID() != scope.ActorID() || a.IntentID() != intent.IntentID() || a.IntentVersion() != intent.Version() || a.IntentDigest() != intent.Digest() || a.WalletBindingID() != o.WalletBindingID || a.WalletBindingVersion() != o.WalletBindingVersion || a.WalletID() != o.WalletID || a.WalletAddress() != o.WalletAddress || a.ChainID() != o.ChainID {
		return apperrors.New(apperrors.CodeApprovalRequired, "Approval does not match the intent.", false, true, true)
	}
	b, err := s.Wallets.FindBindingByID(ctx, scope, o.WalletBindingID)
	if err != nil {
		return err
	}
	if b.BindingID() != o.WalletBindingID || b.Version() != o.WalletBindingVersion || b.OwnerUserID() != o.UserID || b.Provider() != o.EffectiveWalletProvider() || b.ProviderUserReference() != o.ProviderUserReference || b.WalletID() != o.WalletID || b.Address() != o.WalletAddress || b.ChainID() != o.ChainID || b.Network() != o.Network {
		return apperrors.New(apperrors.CodeWalletMismatch, "Wallet binding has changed.", false, true, true)
	}
	return b.EnsureAuthorizable(scope.ActorID())
}
