package postgres

import (
	"context"
	stderrors "errors"

	"github.com/jackc/pgx/v5"

	"github.com/deseti/wizpay-mcp/internal/auth"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/storage/postgres/dbsqlc"
)

func (s *Store) CreateTenant(ctx context.Context, tenant storage.Tenant) (storage.Tenant, error) {
	if tenant.Validate() != nil {
		return storage.Tenant{}, apperrors.New(apperrors.CodeValidationError, "Tenant is invalid.", false, true, true)
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return storage.Tenant{}, err
	}
	defer cancel()
	row, err := s.queries.CreateTenant(bounded, dbsqlc.CreateTenantParams{TenantID: tenant.TenantID, CreatedAt: dbTime(tenant.CreatedAt)})
	if err != nil {
		return storage.Tenant{}, mapDatabaseError(err)
	}
	return storage.Tenant{TenantID: row.TenantID, CreatedAt: domainTime(row.CreatedAt)}, nil
}

func (s *Store) CreateIdentity(ctx context.Context, scope storage.Scope, identity auth.Identity) (auth.Identity, error) {
	if err := scope.Validate(); err != nil {
		return auth.Identity{}, err
	}
	if err := identity.Validate(); err != nil {
		return auth.Identity{}, err
	}
	if identity.UserID() != scope.ActorID() {
		return auth.Identity{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Identity is not accessible.", false, true, true)
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return auth.Identity{}, err
	}
	defer cancel()
	_, err = s.queries.CreateIdentity(bounded, dbsqlc.CreateIdentityParams{TenantID: scope.TenantID(), UserID: identity.UserID(), Provider: identity.Provider(), ProviderSubject: identity.ProviderSubject(), Status: string(identity.Status()), CreatedAt: dbTime(s.now().UTC())})
	if err != nil {
		var appErr *apperrors.Error
		mapped := mapDatabaseError(err)
		if !stderrors.As(mapped, &appErr) || appErr.Code != apperrors.CodeExecutionConflict {
			return auth.Identity{}, mapped
		}
		existing, findErr := s.FindIdentityByID(ctx, scope, identity.UserID())
		if findErr != nil {
			return auth.Identity{}, mapped
		}
		if existing.Provider() != identity.Provider() || existing.ProviderSubject() != identity.ProviderSubject() || existing.Status() != identity.Status() {
			return auth.Identity{}, mapped
		}
		return existing, nil
	}
	return identity, nil
}

func (s *Store) FindIdentityByID(ctx context.Context, scope storage.Scope, userID string) (auth.Identity, error) {
	if err := scope.Validate(); err != nil {
		return auth.Identity{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return auth.Identity{}, err
	}
	defer cancel()
	row, err := s.queries.FindIdentityByID(bounded, dbsqlc.FindIdentityByIDParams{TenantID: scope.TenantID(), UserID: userID, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return auth.Identity{}, notFound(apperrors.CodeIdentityNotFound, "Identity is not accessible.")
	}
	if err != nil {
		return auth.Identity{}, mapDatabaseError(err)
	}
	return auth.NewIdentityWithSubject(row.UserID, row.Provider, row.ProviderSubject, auth.IdentityStatus(row.Status))
}

var _ storage.TenantRepository = (*Store)(nil)
var _ storage.IdentityRepository = (*Store)(nil)
