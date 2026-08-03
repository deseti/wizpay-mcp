package auth

import (
	"context"
	"fmt"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

// TrustedRequest binds verified external claims to an eligible persisted
// identity and server-controlled request metadata.
type TrustedRequest struct {
	principal AuthenticatedPrincipal
	identity  Identity
	metadata  RequestMetadata
}

func NewTrustedRequest(principal AuthenticatedPrincipal, identity Identity, metadata RequestMetadata) (TrustedRequest, error) {
	if err := principal.Validate(); err != nil {
		return TrustedRequest{}, fmt.Errorf("invalid authenticated principal: %w", err)
	}
	if err := identity.EnsureAuthorizable(); err != nil {
		return TrustedRequest{}, err
	}
	identityContext, err := NewIdentityContext(identity, metadata)
	if err != nil {
		return TrustedRequest{}, err
	}
	if identity.UserID() != principal.ActorID() || identity.Provider() != principal.IdentityProvider() || identity.ProviderSubject() != principal.ProviderSubject() {
		return TrustedRequest{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Access is forbidden.", false, true, true)
	}
	return TrustedRequest{principal: principal, identity: identity, metadata: identityContext.Metadata()}, nil
}

func (r TrustedRequest) Validate() error {
	_, err := NewTrustedRequest(r.principal, r.identity, r.metadata)
	return err
}
func (r TrustedRequest) Principal() AuthenticatedPrincipal { return r.principal }
func (r TrustedRequest) Identity() Identity                { return r.identity }
func (r TrustedRequest) Metadata() RequestMetadata         { return r.metadata }

type trustedRequestKey struct{}

func WithTrustedRequest(ctx context.Context, request TrustedRequest) context.Context {
	if ctx == nil || request.Validate() != nil {
		return ctx
	}
	return context.WithValue(ctx, trustedRequestKey{}, request)
}

func TrustedRequestFromContext(ctx context.Context) (TrustedRequest, error) {
	if ctx != nil {
		if request, ok := ctx.Value(trustedRequestKey{}).(TrustedRequest); ok && request.Validate() == nil {
			return request, nil
		}
	}
	return TrustedRequest{}, apperrors.New(apperrors.CodeAuthenticationRequired, "Authentication is required.", false, true, false)
}
