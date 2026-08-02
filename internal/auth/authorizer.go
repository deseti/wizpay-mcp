package auth

import "context"

// WalletBindingContext is the minimal wallet eligibility view consumed by a
// future authorizer. The wallet domain implements it without exposing secrets.
type WalletBindingContext interface {
	BindingID() string
	OwnerUserID() string
	EnsureAuthorizable(userID string) error
}

// AuthorizationInput combines already-resolved identity and wallet metadata.
// It is not an approval and grants no execution authority by itself.
type AuthorizationInput struct {
	Identity IdentityContext
	Wallet   WalletBindingContext
}

// Authorizer defines the future identity-plus-wallet decision boundary. Phase
// 2 intentionally provides no implementation.
type Authorizer interface {
	Authorize(context.Context, AuthorizationInput) error
}
