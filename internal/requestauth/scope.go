package requestauth

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/auth"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

// StorageScopeFromContext is the canonical conversion from trusted request
// authority to tenant-scoped persistence authority.
func StorageScopeFromContext(ctx context.Context) (storage.Scope, error) {
	request, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		return storage.Scope{}, err
	}
	principal := request.Principal()
	metadata := request.Metadata()
	return storage.NewScope(principal.TenantID(), principal.ActorID(), metadata.RequestID, metadata.TraceID)
}

// RepositoryResolver adapts Phase 7's existing identity repository to the
// authentication boundary. The resolver never creates or activates identities.
type RepositoryResolver struct{ Repository storage.IdentityRepository }

func (r RepositoryResolver) ResolveIdentity(ctx context.Context, principal auth.AuthenticatedPrincipal) (auth.Identity, error) {
	if r.Repository == nil {
		return auth.Identity{}, authAccessDenied()
	}
	scope, err := storage.NewScope(principal.TenantID(), principal.ActorID(), "auth-resolution", "")
	if err != nil {
		return auth.Identity{}, authAccessDenied()
	}
	identity, err := r.Repository.FindIdentityByID(ctx, scope, principal.ActorID())
	if err != nil || identity.Provider() != principal.IdentityProvider() || identity.ProviderSubject() != principal.ProviderSubject() || identity.UserID() != principal.ActorID() {
		return auth.Identity{}, authAccessDenied()
	}
	if err := identity.EnsureAuthorizable(); err != nil {
		return auth.Identity{}, authAccessDenied()
	}
	return identity, nil
}

func authAccessDenied() error {
	return apperrors.New(apperrors.CodeAuthorizationRequired, "Access is forbidden.", false, true, true)
}
