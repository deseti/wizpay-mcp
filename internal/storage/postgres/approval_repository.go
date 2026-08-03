package postgres

import (
	"context"
	stderrors "errors"

	"github.com/jackc/pgx/v5"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/storage/postgres/dbsqlc"
)

func approvalCreateParams(scope storage.Scope, value approvals.Approval) (dbsqlc.CreateApprovalParams, error) {
	version, err := dbVersion(value.Version())
	if err != nil {
		return dbsqlc.CreateApprovalParams{}, err
	}
	intentVersion, err := dbVersion(value.IntentVersion())
	if err != nil {
		return dbsqlc.CreateApprovalParams{}, err
	}
	bindingVersion, err := dbVersion(value.WalletBindingVersion())
	if err != nil {
		return dbsqlc.CreateApprovalParams{}, err
	}
	operationVersion, err := dbOptionalVersion(value.OperationVersion())
	if err != nil {
		return dbsqlc.CreateApprovalParams{}, err
	}
	lifecycleRevision, err := dbVersion(value.LifecycleRevision())
	if err != nil {
		return dbsqlc.CreateApprovalParams{}, err
	}
	return dbsqlc.CreateApprovalParams{TenantID: scope.TenantID(), ApprovalID: value.ApprovalID(), ApprovalVersion: version, ApprovalRequestID: value.ApprovalRequestID(), IntentID: value.IntentID(), IntentVersion: intentVersion, IntentDigest: value.IntentDigest(), UserID: value.UserID(), WalletBindingID: value.WalletBindingID(), WalletBindingVersion: bindingVersion, WalletID: value.WalletID(), WalletAddress: value.WalletAddress(), ChainID: value.ChainID(), Status: string(value.Status()), Decision: string(value.Decision()), CreatedAt: dbTime(value.CreatedAt()), ExpiresAt: dbTime(value.ExpiresAt()), DecidedAt: dbTime(value.DecidedAt()), ConsumedAt: dbTime(value.ConsumedAt()), OperationKey: dbOptionalString(value.OperationKey()), OperationVersion: operationVersion, LifecycleVersion: lifecycleRevision}, nil
}
func approvalFromRow(row dbsqlc.Approval) (approvals.Approval, error) {
	version, err := domainVersion(row.ApprovalVersion)
	if err != nil {
		return approvals.Approval{}, err
	}
	intentVersion, err := domainVersion(row.IntentVersion)
	if err != nil {
		return approvals.Approval{}, err
	}
	bindingVersion, err := domainVersion(row.WalletBindingVersion)
	if err != nil {
		return approvals.Approval{}, err
	}
	lifecycleRevision, err := domainVersion(row.LifecycleVersion)
	if err != nil {
		return approvals.Approval{}, err
	}
	return approvals.Restore(approvals.RestoreParams{ApprovalID: row.ApprovalID, Version: version, ApprovalRequestID: row.ApprovalRequestID, IntentID: row.IntentID, IntentVersion: intentVersion, IntentDigest: row.IntentDigest, UserID: row.UserID, WalletBindingID: row.WalletBindingID, WalletBindingVersion: bindingVersion, WalletID: row.WalletID, WalletAddress: row.WalletAddress, ChainID: row.ChainID, Status: approvals.Status(row.Status), Decision: approvals.Decision(row.Decision), CreatedAt: domainTime(row.CreatedAt), ExpiresAt: domainTime(row.ExpiresAt), DecidedAt: domainTime(row.DecidedAt), ConsumedAt: domainTime(row.ConsumedAt), OperationKey: domainOptionalString(row.OperationKey), OperationVersion: domainOptionalVersion(row.OperationVersion), LifecycleRevision: lifecycleRevision})
}
func equalApproval(a, b approvals.Approval) bool {
	return a.ApprovalID() == b.ApprovalID() && a.Version() == b.Version() && a.ApprovalRequestID() == b.ApprovalRequestID() && a.IntentID() == b.IntentID() && a.IntentVersion() == b.IntentVersion() && a.IntentDigest() == b.IntentDigest() && a.UserID() == b.UserID() && a.WalletBindingID() == b.WalletBindingID() && a.WalletBindingVersion() == b.WalletBindingVersion() && a.WalletID() == b.WalletID() && a.WalletAddress() == b.WalletAddress() && a.ChainID() == b.ChainID() && a.Status() == b.Status() && a.Decision() == b.Decision() && a.CreatedAt().Equal(b.CreatedAt()) && a.ExpiresAt().Equal(b.ExpiresAt()) && a.DecidedAt().Equal(b.DecidedAt()) && a.ConsumedAt().Equal(b.ConsumedAt()) && a.OperationKey() == b.OperationKey() && a.OperationVersion() == b.OperationVersion() && a.LifecycleRevision() == b.LifecycleRevision()
}
func (s *Store) CreateApproval(ctx context.Context, scope storage.Scope, value approvals.Approval) (storage.CreateApprovalResult, error) {
	if err := scope.Validate(); err != nil {
		return storage.CreateApprovalResult{}, err
	}
	if err := value.Validate(); err != nil {
		return storage.CreateApprovalResult{}, err
	}
	if value.UserID() != scope.ActorID() {
		return storage.CreateApprovalResult{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Approval is not accessible.", false, true, true)
	}
	params, err := approvalCreateParams(scope, value)
	if err != nil {
		return storage.CreateApprovalResult{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return storage.CreateApprovalResult{}, err
	}
	defer cancel()
	row, err := s.queries.CreateApproval(bounded, params)
	if err == nil {
		restored, err := approvalFromRow(row)
		return storage.CreateApprovalResult{Approval: restored, Created: true}, err
	}
	mapped := mapDatabaseError(err)
	var appErr *apperrors.Error
	if !stderrors.As(mapped, &appErr) || appErr.Code != apperrors.CodeExecutionConflict {
		return storage.CreateApprovalResult{}, mapped
	}
	existing, findErr := s.FindApprovalByIntent(ctx, scope, value.IntentID(), value.IntentVersion(), value.IntentDigest())
	if findErr != nil {
		return storage.CreateApprovalResult{}, mapped
	}
	if !equalApproval(existing, value) {
		return storage.CreateApprovalResult{}, apperrors.New(apperrors.CodeExecutionConflict, "Approval conflicts with the existing intent approval.", false, true, true)
	}
	return storage.CreateApprovalResult{Approval: existing, Created: false}, nil
}
func (s *Store) FindApprovalByID(ctx context.Context, scope storage.Scope, id string) (approvals.Approval, error) {
	if err := scope.Validate(); err != nil {
		return approvals.Approval{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return approvals.Approval{}, err
	}
	defer cancel()
	row, err := s.queries.FindApprovalByID(bounded, dbsqlc.FindApprovalByIDParams{TenantID: scope.TenantID(), ApprovalID: id, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return approvals.Approval{}, notFound(apperrors.CodeApprovalNotFound, "Approval is not accessible.")
	}
	if err != nil {
		return approvals.Approval{}, mapDatabaseError(err)
	}
	return approvalFromRow(row)
}
func (s *Store) FindApprovalByIntent(ctx context.Context, scope storage.Scope, intentID string, intentVersion uint64, digest string) (approvals.Approval, error) {
	if err := scope.Validate(); err != nil {
		return approvals.Approval{}, err
	}
	version, err := dbVersion(intentVersion)
	if err != nil {
		return approvals.Approval{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return approvals.Approval{}, err
	}
	defer cancel()
	row, err := s.queries.FindApprovalByIntent(bounded, dbsqlc.FindApprovalByIntentParams{TenantID: scope.TenantID(), IntentID: intentID, IntentVersion: version, IntentDigest: digest, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return approvals.Approval{}, notFound(apperrors.CodeApprovalNotFound, "Approval is not accessible.")
	}
	if err != nil {
		return approvals.Approval{}, mapDatabaseError(err)
	}
	return approvalFromRow(row)
}
func (s *Store) UpdateApproval(ctx context.Context, scope storage.Scope, value approvals.Approval, expectedRevision uint64) (approvals.Approval, error) {
	if err := scope.Validate(); err != nil {
		return approvals.Approval{}, err
	}
	if err := value.Validate(); err != nil {
		return approvals.Approval{}, err
	}
	if value.UserID() != scope.ActorID() {
		return approvals.Approval{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Approval is not accessible.", false, true, true)
	}
	operationVersion, err := dbOptionalVersion(value.OperationVersion())
	if err != nil {
		return approvals.Approval{}, err
	}
	revision, err := dbVersion(value.LifecycleRevision())
	if err != nil {
		return approvals.Approval{}, err
	}
	expected, err := dbVersion(expectedRevision)
	if err != nil {
		return approvals.Approval{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return approvals.Approval{}, err
	}
	defer cancel()
	row, err := s.queries.UpdateApproval(bounded, dbsqlc.UpdateApprovalParams{TenantID: scope.TenantID(), ApprovalID: value.ApprovalID(), Status: string(value.Status()), Decision: string(value.Decision()), DecidedAt: dbTime(value.DecidedAt()), ConsumedAt: dbTime(value.ConsumedAt()), OperationKey: dbOptionalString(value.OperationKey()), OperationVersion: operationVersion, LifecycleVersion: revision, LifecycleVersion_2: expected, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return approvals.Approval{}, apperrors.New(apperrors.CodeExecutionConflict, "Approval changed concurrently.", false, true, true)
	}
	if err != nil {
		return approvals.Approval{}, mapDatabaseError(err)
	}
	return approvalFromRow(row)
}

var _ storage.ApprovalRepository = (*Store)(nil)
