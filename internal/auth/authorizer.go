package auth

import (
	"context"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

// WalletBindingContext is the minimal wallet eligibility view consumed by a
// future authorizer. The wallet domain implements it without exposing secrets.
type WalletBindingContext interface {
	BindingID() string
	OwnerUserID() string
	EnsureAuthorizable(userID string) error
}

// AuthorizationInput combines trusted request authority and optional ownership
// constraints. It is not an approval and grants no execution authority by itself.
type AuthorizationInput struct {
	Request         TrustedRequest
	Permission      Permission
	ResourceOwnerID string
	Wallet          WalletBindingContext
}

// Authorizer defines capability access independently from financial approval.
type Authorizer interface {
	Authorize(context.Context, AuthorizationInput) error
}

type PermissionAuthorizer struct{}

func NewPermissionAuthorizer() PermissionAuthorizer { return PermissionAuthorizer{} }

func (PermissionAuthorizer) Authorize(ctx context.Context, input AuthorizationInput) error {
	if ctx == nil || input.Request.Validate() != nil || !input.Permission.Valid() {
		return authorizationDenied()
	}
	principal := input.Request.Principal()
	if !principal.HasPermission(input.Permission) {
		return authorizationDenied()
	}
	if input.ResourceOwnerID != "" && input.ResourceOwnerID != principal.ActorID() {
		return authorizationDenied()
	}
	if input.Wallet != nil {
		if input.Wallet.OwnerUserID() != principal.ActorID() || input.Wallet.EnsureAuthorizable(principal.ActorID()) != nil {
			return authorizationDenied()
		}
	}
	return nil
}

func authorizationDenied() error {
	return apperrors.New(apperrors.CodeAuthorizationRequired, "Access is forbidden.", false, true, true)
}
