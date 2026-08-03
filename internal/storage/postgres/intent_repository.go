package postgres

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"reflect"

	"github.com/jackc/pgx/v5"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/storage/postgres/dbsqlc"
)

func intentCreateParams(scope storage.Scope, value intents.Intent) (dbsqlc.CreateIntentParams, error) {
	version, err := dbVersion(value.Version())
	if err != nil {
		return dbsqlc.CreateIntentParams{}, err
	}
	owner := value.Ownership()
	bindingVersion, err := dbVersion(owner.WalletBindingVersion)
	if err != nil {
		return dbsqlc.CreateIntentParams{}, err
	}
	route := value.Route()
	routeVersion, err := dbVersion(route.Version)
	if err != nil {
		return dbsqlc.CreateIntentParams{}, err
	}
	financial, err := json.Marshal(value.Financial())
	if err != nil {
		return dbsqlc.CreateIntentParams{}, err
	}
	lifecycleRevision, err := dbVersion(value.LifecycleRevision())
	if err != nil {
		return dbsqlc.CreateIntentParams{}, err
	}
	var operationKey *string
	var operationVersion *int64
	if value.Status() != intents.StatusDraft {
		operation, err := intents.NewOperationIdentity(value)
		if err != nil {
			return dbsqlc.CreateIntentParams{}, err
		}
		operationKey = dbOptionalString(operation.OperationKey())
		operationVersion, err = dbOptionalVersion(operation.Version())
		if err != nil {
			return dbsqlc.CreateIntentParams{}, err
		}
	}
	return dbsqlc.CreateIntentParams{TenantID: scope.TenantID(), IntentID: value.IntentID(), IntentVersion: version, ClientRequestID: value.ClientRequestID(), Nonce: value.Nonce(), IntentType: string(value.Type()), UserID: owner.UserID, IdentityProvider: owner.IdentityProvider, ProviderUserReference: owner.ProviderUserReference, WalletBindingID: owner.WalletBindingID, WalletBindingVersion: bindingVersion, WalletID: owner.WalletID, WalletAddress: owner.WalletAddress, ChainID: owner.ChainID, Network: owner.Network, Financial: financial, RouteType: string(route.Type), RouteReference: route.Reference, RouteVersion: routeVersion, ConstraintDeadline: dbTime(value.Constraints().Deadline), PolicyReference: value.Constraints().PolicyReference, CreatedAt: dbTime(value.CreatedAt()), ExpiresAt: dbTime(value.ExpiresAt()), Status: string(value.Status()), IntentDigest: dbOptionalString(value.Digest()), OperationKey: operationKey, OperationVersion: operationVersion, LifecycleVersion: lifecycleRevision}, nil
}

func intentFromRow(row dbsqlc.Intent) (intents.Intent, error) {
	version, err := domainVersion(row.IntentVersion)
	if err != nil {
		return intents.Intent{}, err
	}
	bindingVersion, err := domainVersion(row.WalletBindingVersion)
	if err != nil {
		return intents.Intent{}, err
	}
	routeVersion, err := domainVersion(row.RouteVersion)
	if err != nil {
		return intents.Intent{}, err
	}
	var financial intents.FinancialParameters
	if err := json.Unmarshal(row.Financial, &financial); err != nil {
		return intents.Intent{}, apperrors.Wrap(apperrors.CodeIntentMutated, "Persisted intent material is invalid.", false, true, true, err)
	}
	params := intents.Params{IntentID: row.IntentID, Version: version, ClientRequestID: row.ClientRequestID, Nonce: row.Nonce, Type: intents.Type(row.IntentType), Ownership: intents.Ownership{UserID: row.UserID, IdentityProvider: row.IdentityProvider, ProviderUserReference: row.ProviderUserReference, WalletBindingID: row.WalletBindingID, WalletBindingVersion: bindingVersion, WalletID: row.WalletID, WalletAddress: row.WalletAddress, ChainID: row.ChainID, Network: row.Network}, Financial: financial, Route: intents.Route{Type: intents.RouteType(row.RouteType), Reference: row.RouteReference, Version: routeVersion}, Constraints: intents.Constraints{Deadline: domainTime(row.ConstraintDeadline), PolicyReference: row.PolicyReference}, CreatedAt: domainTime(row.CreatedAt), ExpiresAt: domainTime(row.ExpiresAt)}
	lifecycleRevision, err := domainVersion(row.LifecycleVersion)
	if err != nil {
		return intents.Intent{}, err
	}
	return intents.Restore(params, intents.Status(row.Status), domainOptionalString(row.IntentDigest), lifecycleRevision)
}

func (s *Store) CreateIntent(ctx context.Context, scope storage.Scope, value intents.Intent) (storage.CreateIntentResult, error) {
	if err := scope.Validate(); err != nil {
		return storage.CreateIntentResult{}, err
	}
	if err := value.Validate(); err != nil {
		return storage.CreateIntentResult{}, err
	}
	if value.Ownership().UserID != scope.ActorID() {
		return storage.CreateIntentResult{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Intent is not accessible.", false, true, true)
	}
	params, err := intentCreateParams(scope, value)
	if err != nil {
		return storage.CreateIntentResult{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return storage.CreateIntentResult{}, err
	}
	defer cancel()
	row, err := s.queries.CreateIntent(bounded, params)
	if err == nil {
		restored, err := intentFromRow(row)
		return storage.CreateIntentResult{Intent: restored, Created: true}, err
	}
	mapped := mapDatabaseError(err)
	var appErr *apperrors.Error
	if !stderrors.As(mapped, &appErr) || appErr.Code != apperrors.CodeExecutionConflict {
		return storage.CreateIntentResult{}, mapped
	}
	existing, findErr := s.FindIntentByClientRequestID(ctx, scope, value.ClientRequestID())
	if findErr != nil {
		return storage.CreateIntentResult{}, mapped
	}
	if !equalIntent(existing, value) {
		return storage.CreateIntentResult{}, apperrors.New(apperrors.CodeIntentMutated, "Intent idempotency key conflicts with different material.", false, true, true)
	}
	return storage.CreateIntentResult{Intent: existing, Created: false}, nil
}
func equalIntent(a, b intents.Intent) bool {
	return a.IntentID() == b.IntentID() && a.Version() == b.Version() && a.ClientRequestID() == b.ClientRequestID() && a.Nonce() == b.Nonce() && a.Type() == b.Type() && reflect.DeepEqual(a.Ownership(), b.Ownership()) && reflect.DeepEqual(a.Financial(), b.Financial()) && reflect.DeepEqual(a.Route(), b.Route()) && reflect.DeepEqual(a.Constraints(), b.Constraints()) && a.CreatedAt().Equal(b.CreatedAt()) && a.ExpiresAt().Equal(b.ExpiresAt()) && a.Status() == b.Status() && a.Digest() == b.Digest() && a.LifecycleRevision() == b.LifecycleRevision()
}

func (s *Store) FindIntentByID(ctx context.Context, scope storage.Scope, id string) (intents.Intent, error) {
	if err := scope.Validate(); err != nil {
		return intents.Intent{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return intents.Intent{}, err
	}
	defer cancel()
	row, err := s.queries.FindIntentByID(bounded, dbsqlc.FindIntentByIDParams{TenantID: scope.TenantID(), IntentID: id, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return intents.Intent{}, notFound(apperrors.CodeIntentNotFound, "Intent is not accessible.")
	}
	if err != nil {
		return intents.Intent{}, mapDatabaseError(err)
	}
	return intentFromRow(row)
}
func (s *Store) FindIntentByClientRequestID(ctx context.Context, scope storage.Scope, id string) (intents.Intent, error) {
	if err := scope.Validate(); err != nil {
		return intents.Intent{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return intents.Intent{}, err
	}
	defer cancel()
	row, err := s.queries.FindIntentByClientRequestID(bounded, dbsqlc.FindIntentByClientRequestIDParams{TenantID: scope.TenantID(), ClientRequestID: id, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return intents.Intent{}, notFound(apperrors.CodeIntentNotFound, "Intent is not accessible.")
	}
	if err != nil {
		return intents.Intent{}, mapDatabaseError(err)
	}
	return intentFromRow(row)
}
func (s *Store) FindIntentByOperationKey(ctx context.Context, scope storage.Scope, key string, version uint64) (intents.Intent, error) {
	if err := scope.Validate(); err != nil {
		return intents.Intent{}, err
	}
	dbv, err := dbVersion(version)
	if err != nil {
		return intents.Intent{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return intents.Intent{}, err
	}
	defer cancel()
	row, err := s.queries.FindIntentByOperationKey(bounded, dbsqlc.FindIntentByOperationKeyParams{TenantID: scope.TenantID(), OperationKey: &key, OperationVersion: &dbv, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return intents.Intent{}, notFound(apperrors.CodeIntentNotFound, "Intent is not accessible.")
	}
	if err != nil {
		return intents.Intent{}, mapDatabaseError(err)
	}
	return intentFromRow(row)
}

func (s *Store) FreezeIntent(ctx context.Context, scope storage.Scope, value intents.Intent, expectedRevision uint64) (intents.Intent, error) {
	if err := scope.Validate(); err != nil {
		return intents.Intent{}, err
	}
	if err := value.Validate(); err != nil {
		return intents.Intent{}, err
	}
	if value.Status() != intents.StatusCreated {
		return intents.Intent{}, apperrors.New(apperrors.CodeValidationError, "Only a CREATED intent may be frozen.", false, true, true)
	}
	if value.Ownership().UserID != scope.ActorID() {
		return intents.Intent{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Intent is not accessible.", false, true, true)
	}
	if expectedRevision == ^uint64(0) || value.LifecycleRevision() != expectedRevision+1 {
		return intents.Intent{}, apperrors.New(apperrors.CodeExecutionConflict, "Intent revision must advance exactly once.", false, true, true)
	}
	operation, err := intents.NewOperationIdentity(value)
	if err != nil {
		return intents.Intent{}, err
	}
	revision, err := dbVersion(value.LifecycleRevision())
	if err != nil {
		return intents.Intent{}, err
	}
	expected, err := dbVersion(expectedRevision)
	if err != nil {
		return intents.Intent{}, err
	}
	operationVersion, err := dbVersion(operation.Version())
	if err != nil {
		return intents.Intent{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return intents.Intent{}, err
	}
	defer cancel()
	row, err := s.queries.FreezeIntent(bounded, dbsqlc.FreezeIntentParams{TenantID: scope.TenantID(), IntentID: value.IntentID(), IntentDigest: dbOptionalString(value.Digest()), OperationKey: dbOptionalString(operation.OperationKey()), OperationVersion: &operationVersion, LifecycleVersion: revision, LifecycleVersion_2: expected, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return intents.Intent{}, apperrors.New(apperrors.CodeExecutionConflict, "Intent changed concurrently.", false, true, true)
	}
	if err != nil {
		return intents.Intent{}, mapDatabaseError(err)
	}
	return intentFromRow(row)
}

func (s *Store) UpdateIntent(ctx context.Context, scope storage.Scope, value intents.Intent, expectedRevision uint64) (intents.Intent, error) {
	if err := scope.Validate(); err != nil {
		return intents.Intent{}, err
	}
	if err := value.Validate(); err != nil {
		return intents.Intent{}, err
	}
	if value.Ownership().UserID != scope.ActorID() {
		return intents.Intent{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Intent is not accessible.", false, true, true)
	}
	revision, err := dbVersion(value.LifecycleRevision())
	if err != nil {
		return intents.Intent{}, err
	}
	expected, err := dbVersion(expectedRevision)
	if err != nil {
		return intents.Intent{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return intents.Intent{}, err
	}
	defer cancel()
	row, err := s.queries.UpdateIntent(bounded, dbsqlc.UpdateIntentParams{TenantID: scope.TenantID(), IntentID: value.IntentID(), Status: string(value.Status()), LifecycleVersion: revision, LifecycleVersion_2: expected, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return intents.Intent{}, apperrors.New(apperrors.CodeExecutionConflict, "Intent changed concurrently.", false, true, true)
	}
	if err != nil {
		return intents.Intent{}, mapDatabaseError(err)
	}
	return intentFromRow(row)
}

var _ storage.IntentRepository = (*Store)(nil)
