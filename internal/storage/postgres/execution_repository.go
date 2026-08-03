package postgres

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/storage/postgres/dbsqlc"
)

func requestCreateParams(scope storage.Scope, value execution.Request) (dbsqlc.CreateExecutionRequestParams, error) {
	requestVersion, err := dbVersion(value.Version())
	if err != nil {
		return dbsqlc.CreateExecutionRequestParams{}, err
	}
	operationVersion, err := dbVersion(value.OperationVersion())
	if err != nil {
		return dbsqlc.CreateExecutionRequestParams{}, err
	}
	intentVersion, err := dbVersion(value.IntentVersion())
	if err != nil {
		return dbsqlc.CreateExecutionRequestParams{}, err
	}
	approvalVersion, err := dbVersion(value.ApprovalVersion())
	if err != nil {
		return dbsqlc.CreateExecutionRequestParams{}, err
	}
	policyVersion, err := dbVersion(value.PolicyVersion())
	if err != nil {
		return dbsqlc.CreateExecutionRequestParams{}, err
	}
	return dbsqlc.CreateExecutionRequestParams{TenantID: scope.TenantID(), RequestID: value.RequestID(), RequestKey: value.RequestKey(), RequestVersion: requestVersion, ExecutionID: value.ExecutionID(), OperationKey: value.OperationKey(), OperationVersion: operationVersion, IntentID: value.IntentID(), IntentVersion: intentVersion, IntentDigest: value.IntentDigest(), ApprovalID: value.ApprovalID(), ApprovalVersion: approvalVersion, UserID: scope.ActorID(), PolicyID: value.PolicyID(), PolicyVersion: policyVersion, PolicyEvaluationKey: value.PolicyEvaluationKey(), PolicyEvaluationStage: string(value.PolicyEvaluationStage()), PolicyEvaluatedAt: dbTime(value.PolicyEvaluatedAt()), CreatedAt: dbTime(value.CreatedAt())}, nil
}
func executionCreateParams(scope storage.Scope, value execution.Execution) (dbsqlc.CreateExecutionParams, error) {
	revision, err := dbVersion(value.Revision())
	if err != nil {
		return dbsqlc.CreateExecutionParams{}, err
	}
	failure, _ := value.Failure()
	recovery, _ := value.Recovery()
	return dbsqlc.CreateExecutionParams{TenantID: scope.TenantID(), ExecutionID: value.ExecutionID(), RequestID: value.Request().RequestID(), Status: string(value.Status()), Revision: revision, CreatedAt: dbTime(value.CreatedAt()), UpdatedAt: dbTime(value.UpdatedAt()), FailureCode: failure.Code, FailureEligibility: string(failure.Eligibility), FailureRecoveryTarget: string(failure.RecoveryTarget), FailedFromStatus: string(value.FailedFrom()), RecoveryReasonCode: recovery.ReasonCode, RecoveryFromStatus: string(recovery.FromStatus), RecoveryTargetStatus: string(recovery.Target)}, nil
}

type executionRecord struct {
	ExecutionID, RequestID, Status, FailureCode, FailureEligibility, FailureRecoveryTarget, FailedFromStatus, RecoveryReasonCode, RecoveryFromStatus, RecoveryTargetStatus, RequestKey, OperationKey, IntentID, IntentDigest, ApprovalID, PolicyID, PolicyEvaluationKey, PolicyEvaluationStage string
	Revision, RequestVersion, OperationVersion, IntentVersion, ApprovalVersion, PolicyVersion                                                                                                                                                                                                  int64
	CreatedAt, UpdatedAt, PolicyEvaluatedAt, RequestCreatedAt                                                                                                                                                                                                                                  pgtype.Timestamptz
}

func restoreExecutionRecord(row executionRecord) (execution.Execution, error) {
	requestVersion, err := domainVersion(row.RequestVersion)
	if err != nil {
		return execution.Execution{}, err
	}
	operationVersion, err := domainVersion(row.OperationVersion)
	if err != nil {
		return execution.Execution{}, err
	}
	intentVersion, err := domainVersion(row.IntentVersion)
	if err != nil {
		return execution.Execution{}, err
	}
	approvalVersion, err := domainVersion(row.ApprovalVersion)
	if err != nil {
		return execution.Execution{}, err
	}
	policyVersion, err := domainVersion(row.PolicyVersion)
	if err != nil {
		return execution.Execution{}, err
	}
	revision, err := domainVersion(row.Revision)
	if err != nil {
		return execution.Execution{}, err
	}
	request, err := execution.RestoreRequest(execution.RestoreRequestParams{RequestID: row.RequestID, RequestKey: row.RequestKey, Version: requestVersion, ExecutionID: row.ExecutionID, OperationKey: row.OperationKey, OperationVersion: operationVersion, IntentID: row.IntentID, IntentVersion: intentVersion, IntentDigest: row.IntentDigest, ApprovalID: row.ApprovalID, ApprovalVersion: approvalVersion, PolicyID: row.PolicyID, PolicyVersion: policyVersion, PolicyEvaluationKey: row.PolicyEvaluationKey, PolicyEvaluationStage: policies.EvaluationStage(row.PolicyEvaluationStage), PolicyEvaluatedAt: domainTime(row.PolicyEvaluatedAt), CreatedAt: domainTime(row.RequestCreatedAt)})
	if err != nil {
		return execution.Execution{}, err
	}
	failure := execution.Failure{Code: row.FailureCode, Eligibility: execution.RecoveryEligibility(row.FailureEligibility), RecoveryTarget: execution.Status(row.FailureRecoveryTarget)}
	recovery := execution.Recovery{ReasonCode: row.RecoveryReasonCode, FromStatus: execution.Status(row.RecoveryFromStatus), Target: execution.Status(row.RecoveryTargetStatus)}
	return execution.Restore(execution.RestoreExecutionParams{Request: request, Status: execution.Status(row.Status), Revision: revision, CreatedAt: domainTime(row.CreatedAt), UpdatedAt: domainTime(row.UpdatedAt), Failure: failure, FailedFrom: execution.Status(row.FailedFromStatus), Recovery: recovery})
}
func recordByID(row dbsqlc.FindExecutionByIDRow) executionRecord {
	return executionRecord{ExecutionID: row.ExecutionID, RequestID: row.RequestID, Status: row.Status, Revision: row.Revision, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, FailureCode: row.FailureCode, FailureEligibility: row.FailureEligibility, FailureRecoveryTarget: row.FailureRecoveryTarget, FailedFromStatus: row.FailedFromStatus, RecoveryReasonCode: row.RecoveryReasonCode, RecoveryFromStatus: row.RecoveryFromStatus, RecoveryTargetStatus: row.RecoveryTargetStatus, RequestKey: row.RequestKey, RequestVersion: row.RequestVersion, OperationKey: row.OperationKey, OperationVersion: row.OperationVersion, IntentID: row.IntentID, IntentVersion: row.IntentVersion, IntentDigest: row.IntentDigest, ApprovalID: row.ApprovalID, ApprovalVersion: row.ApprovalVersion, PolicyID: row.PolicyID, PolicyVersion: row.PolicyVersion, PolicyEvaluationKey: row.PolicyEvaluationKey, PolicyEvaluationStage: row.PolicyEvaluationStage, PolicyEvaluatedAt: row.PolicyEvaluatedAt, RequestCreatedAt: row.RequestCreatedAt}
}
func recordByOperation(row dbsqlc.FindExecutionByOperationKeyRow) executionRecord {
	return executionRecord{ExecutionID: row.ExecutionID, RequestID: row.RequestID, Status: row.Status, Revision: row.Revision, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, FailureCode: row.FailureCode, FailureEligibility: row.FailureEligibility, FailureRecoveryTarget: row.FailureRecoveryTarget, FailedFromStatus: row.FailedFromStatus, RecoveryReasonCode: row.RecoveryReasonCode, RecoveryFromStatus: row.RecoveryFromStatus, RecoveryTargetStatus: row.RecoveryTargetStatus, RequestKey: row.RequestKey, RequestVersion: row.RequestVersion, OperationKey: row.OperationKey, OperationVersion: row.OperationVersion, IntentID: row.IntentID, IntentVersion: row.IntentVersion, IntentDigest: row.IntentDigest, ApprovalID: row.ApprovalID, ApprovalVersion: row.ApprovalVersion, PolicyID: row.PolicyID, PolicyVersion: row.PolicyVersion, PolicyEvaluationKey: row.PolicyEvaluationKey, PolicyEvaluationStage: row.PolicyEvaluationStage, PolicyEvaluatedAt: row.PolicyEvaluatedAt, RequestCreatedAt: row.RequestCreatedAt}
}
func recordByRequest(row dbsqlc.FindExecutionByRequestKeyRow) executionRecord {
	return executionRecord{ExecutionID: row.ExecutionID, RequestID: row.RequestID, Status: row.Status, Revision: row.Revision, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, FailureCode: row.FailureCode, FailureEligibility: row.FailureEligibility, FailureRecoveryTarget: row.FailureRecoveryTarget, FailedFromStatus: row.FailedFromStatus, RecoveryReasonCode: row.RecoveryReasonCode, RecoveryFromStatus: row.RecoveryFromStatus, RecoveryTargetStatus: row.RecoveryTargetStatus, RequestKey: row.RequestKey, RequestVersion: row.RequestVersion, OperationKey: row.OperationKey, OperationVersion: row.OperationVersion, IntentID: row.IntentID, IntentVersion: row.IntentVersion, IntentDigest: row.IntentDigest, ApprovalID: row.ApprovalID, ApprovalVersion: row.ApprovalVersion, PolicyID: row.PolicyID, PolicyVersion: row.PolicyVersion, PolicyEvaluationKey: row.PolicyEvaluationKey, PolicyEvaluationStage: row.PolicyEvaluationStage, PolicyEvaluatedAt: row.PolicyEvaluatedAt, RequestCreatedAt: row.RequestCreatedAt}
}

type executionRequestIdentity struct {
	requestID, requestKey, executionID, operationKey, intentID, intentDigest, approvalID, policyID, policyEvaluationKey string
	version, operationVersion, intentVersion, approvalVersion, policyVersion                                            uint64
	policyEvaluationStage                                                                                               policies.EvaluationStage
	policyEvaluatedAt, createdAt                                                                                        time.Time
}

func executionRequestIdentityOf(value execution.Request) executionRequestIdentity {
	return executionRequestIdentity{
		requestID: value.RequestID(), requestKey: value.RequestKey(), version: value.Version(), executionID: value.ExecutionID(),
		operationKey: value.OperationKey(), operationVersion: value.OperationVersion(), intentID: value.IntentID(), intentVersion: value.IntentVersion(),
		intentDigest: value.IntentDigest(), approvalID: value.ApprovalID(), approvalVersion: value.ApprovalVersion(), policyID: value.PolicyID(),
		policyVersion: value.PolicyVersion(), policyEvaluationKey: value.PolicyEvaluationKey(), policyEvaluationStage: value.PolicyEvaluationStage(),
		policyEvaluatedAt: value.PolicyEvaluatedAt(), createdAt: value.CreatedAt(),
	}
}

// equalExecutionRequest is the single retry-identity comparison used by every
// execution creation path. Mutable execution lifecycle fields are excluded.
func equalExecutionRequest(a, b execution.Request) bool {
	return executionRequestIdentityOf(a) == executionRequestIdentityOf(b)
}

func (s *Store) CreateExecution(ctx context.Context, scope storage.Scope, value execution.Execution) (storage.CreateExecutionResult, error) {
	if err := scope.Validate(); err != nil {
		return storage.CreateExecutionResult{}, err
	}
	if err := value.Validate(); err != nil {
		return storage.CreateExecutionResult{}, err
	}
	requestParams, err := requestCreateParams(scope, value.Request())
	if err != nil {
		return storage.CreateExecutionResult{}, err
	}
	executionParams, err := executionCreateParams(scope, value)
	if err != nil {
		return storage.CreateExecutionResult{}, err
	}
	err = s.withTx(ctx, func(txctx context.Context, q *dbsqlc.Queries) error {
		if _, err := q.CreateExecutionRequest(txctx, requestParams); err != nil {
			return err
		}
		_, err := q.CreateExecution(txctx, executionParams)
		return err
	})
	if err == nil {
		return storage.CreateExecutionResult{Execution: value, Created: true}, nil
	}
	var appErr *apperrors.Error
	if !stderrors.As(err, &appErr) || appErr.Code != apperrors.CodeExecutionConflict {
		return storage.CreateExecutionResult{}, err
	}
	existing, findErr := s.FindExecutionByOperationKey(ctx, scope, value.Request().OperationKey(), value.Request().OperationVersion())
	if findErr != nil {
		return storage.CreateExecutionResult{}, err
	}
	if !equalExecutionRequest(existing.Request(), value.Request()) {
		return storage.CreateExecutionResult{}, apperrors.New(apperrors.CodeExecutionConflict, "Operation already has a different execution request.", false, true, true)
	}
	return storage.CreateExecutionResult{Execution: existing, Created: false}, nil
}
func (s *Store) FindExecutionByID(ctx context.Context, scope storage.Scope, id string) (execution.Execution, error) {
	if err := scope.Validate(); err != nil {
		return execution.Execution{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return execution.Execution{}, err
	}
	defer cancel()
	row, err := s.queries.FindExecutionByID(bounded, dbsqlc.FindExecutionByIDParams{TenantID: scope.TenantID(), ExecutionID: id, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return execution.Execution{}, notFound(apperrors.CodeExecutionNotFound, "Execution is not accessible.")
	}
	if err != nil {
		return execution.Execution{}, mapDatabaseError(err)
	}
	return restoreExecutionRecord(recordByID(row))
}
func (s *Store) FindExecutionByRequestKey(ctx context.Context, scope storage.Scope, key string, version uint64) (execution.Execution, error) {
	if err := scope.Validate(); err != nil {
		return execution.Execution{}, err
	}
	dbv, err := dbVersion(version)
	if err != nil {
		return execution.Execution{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return execution.Execution{}, err
	}
	defer cancel()
	row, err := s.queries.FindExecutionByRequestKey(bounded, dbsqlc.FindExecutionByRequestKeyParams{TenantID: scope.TenantID(), RequestKey: key, RequestVersion: dbv, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return execution.Execution{}, notFound(apperrors.CodeExecutionNotFound, "Execution is not accessible.")
	}
	if err != nil {
		return execution.Execution{}, mapDatabaseError(err)
	}
	return restoreExecutionRecord(recordByRequest(row))
}
func (s *Store) FindExecutionByOperationKey(ctx context.Context, scope storage.Scope, key string, version uint64) (execution.Execution, error) {
	if err := scope.Validate(); err != nil {
		return execution.Execution{}, err
	}
	dbv, err := dbVersion(version)
	if err != nil {
		return execution.Execution{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return execution.Execution{}, err
	}
	defer cancel()
	row, err := s.queries.FindExecutionByOperationKey(bounded, dbsqlc.FindExecutionByOperationKeyParams{TenantID: scope.TenantID(), OperationKey: key, OperationVersion: dbv, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return execution.Execution{}, notFound(apperrors.CodeExecutionNotFound, "Execution is not accessible.")
	}
	if err != nil {
		return execution.Execution{}, mapDatabaseError(err)
	}
	return restoreExecutionRecord(recordByOperation(row))
}
func (s *Store) UpdateExecution(ctx context.Context, scope storage.Scope, value execution.Execution, expected uint64) (execution.Execution, error) {
	if err := scope.Validate(); err != nil {
		return execution.Execution{}, err
	}
	if err := value.Validate(); err != nil {
		return execution.Execution{}, err
	}
	if expected == ^uint64(0) || value.Revision() != expected+1 {
		return execution.Execution{}, apperrors.New(apperrors.CodeExecutionConflict, "Execution revision must advance exactly once.", false, true, true)
	}
	revision, err := dbVersion(value.Revision())
	if err != nil {
		return execution.Execution{}, err
	}
	expectedRevision, err := dbVersion(expected)
	if err != nil {
		return execution.Execution{}, err
	}
	failure, _ := value.Failure()
	recovery, _ := value.Recovery()
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return execution.Execution{}, err
	}
	defer cancel()
	row, err := s.queries.UpdateExecution(bounded, dbsqlc.UpdateExecutionParams{TenantID: scope.TenantID(), ExecutionID: value.ExecutionID(), Status: string(value.Status()), Revision: revision, UpdatedAt: dbTime(value.UpdatedAt()), FailureCode: failure.Code, FailureEligibility: string(failure.Eligibility), FailureRecoveryTarget: string(failure.RecoveryTarget), FailedFromStatus: string(value.FailedFrom()), RecoveryReasonCode: recovery.ReasonCode, RecoveryFromStatus: string(recovery.FromStatus), RecoveryTargetStatus: string(recovery.Target), Revision_2: expectedRevision, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return execution.Execution{}, apperrors.New(apperrors.CodeExecutionConflict, "Execution changed concurrently.", false, true, true)
	}
	if err != nil {
		return execution.Execution{}, mapDatabaseError(err)
	}
	record := executionRecord{ExecutionID: row.ExecutionID, RequestID: row.RequestID, Status: row.Status, Revision: row.Revision, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, FailureCode: row.FailureCode, FailureEligibility: row.FailureEligibility, FailureRecoveryTarget: row.FailureRecoveryTarget, FailedFromStatus: row.FailedFromStatus, RecoveryReasonCode: row.RecoveryReasonCode, RecoveryFromStatus: row.RecoveryFromStatus, RecoveryTargetStatus: row.RecoveryTargetStatus, RequestKey: value.Request().RequestKey(), RequestVersion: int64(value.Request().Version()), OperationKey: value.Request().OperationKey(), OperationVersion: int64(value.Request().OperationVersion()), IntentID: value.Request().IntentID(), IntentVersion: int64(value.Request().IntentVersion()), IntentDigest: value.Request().IntentDigest(), ApprovalID: value.Request().ApprovalID(), ApprovalVersion: int64(value.Request().ApprovalVersion()), PolicyID: value.Request().PolicyID(), PolicyVersion: int64(value.Request().PolicyVersion()), PolicyEvaluationKey: value.Request().PolicyEvaluationKey(), PolicyEvaluationStage: string(value.Request().PolicyEvaluationStage()), PolicyEvaluatedAt: dbTime(value.Request().PolicyEvaluatedAt()), RequestCreatedAt: dbTime(value.Request().CreatedAt())}
	return restoreExecutionRecord(record)
}

var _ storage.ExecutionRepository = (*Store)(nil)
