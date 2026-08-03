package postgres

import (
	"context"
	stderrors "errors"

	"github.com/jackc/pgx/v5"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/audit"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/storage/postgres/dbsqlc"
)

func (s *Store) CreateIntentWithAudit(ctx context.Context, scope storage.Scope, value intents.Intent, record audit.Record) (storage.CreateIntentResult, error) {
	if err := scope.Validate(); err != nil {
		return storage.CreateIntentResult{}, err
	}
	if err := value.Validate(); err != nil {
		return storage.CreateIntentResult{}, err
	}
	if value.Ownership().UserID != scope.ActorID() {
		return storage.CreateIntentResult{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Intent owner does not match the trusted request scope.", false, true, true)
	}
	params, err := intentCreateParams(scope, value)
	if err != nil {
		return storage.CreateIntentResult{}, err
	}
	auditValue, err := auditParams(scope, record)
	if err != nil {
		return storage.CreateIntentResult{}, err
	}
	var row dbsqlc.Intent
	err = s.withTx(ctx, func(txctx context.Context, q *dbsqlc.Queries) error {
		var err error
		row, err = q.CreateIntent(txctx, params)
		if err != nil {
			return err
		}
		return q.AppendAudit(txctx, auditValue)
	})
	if err == nil {
		restored, err := intentFromRow(row)
		return storage.CreateIntentResult{Intent: restored, Created: true}, err
	}
	var appErr *apperrors.Error
	if !stderrors.As(err, &appErr) || appErr.Code != apperrors.CodeExecutionConflict {
		return storage.CreateIntentResult{}, err
	}
	existing, findErr := s.FindIntentByClientRequestID(ctx, scope, value.ClientRequestID())
	if findErr == nil && equalIntent(existing, value) {
		return storage.CreateIntentResult{Intent: existing, Created: false}, nil
	}
	return storage.CreateIntentResult{}, err
}
func (s *Store) CreateApprovalWithAudit(ctx context.Context, scope storage.Scope, value approvals.Approval, record audit.Record) (storage.CreateApprovalResult, error) {
	if err := scope.Validate(); err != nil {
		return storage.CreateApprovalResult{}, err
	}
	if err := value.Validate(); err != nil {
		return storage.CreateApprovalResult{}, err
	}
	if value.UserID() != scope.ActorID() {
		return storage.CreateApprovalResult{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Approval owner does not match the trusted request scope.", false, true, true)
	}
	params, err := approvalCreateParams(scope, value)
	if err != nil {
		return storage.CreateApprovalResult{}, err
	}
	auditValue, err := auditParams(scope, record)
	if err != nil {
		return storage.CreateApprovalResult{}, err
	}
	var row dbsqlc.Approval
	err = s.withTx(ctx, func(txctx context.Context, q *dbsqlc.Queries) error {
		var err error
		row, err = q.CreateApproval(txctx, params)
		if err != nil {
			return err
		}
		return q.AppendAudit(txctx, auditValue)
	})
	if err == nil {
		restored, err := approvalFromRow(row)
		return storage.CreateApprovalResult{Approval: restored, Created: true}, err
	}
	var appErr *apperrors.Error
	if !stderrors.As(err, &appErr) || appErr.Code != apperrors.CodeExecutionConflict {
		return storage.CreateApprovalResult{}, err
	}
	existing, findErr := s.FindApprovalByIntent(ctx, scope, value.IntentID(), value.IntentVersion(), value.IntentDigest())
	if findErr == nil && equalApproval(existing, value) {
		return storage.CreateApprovalResult{Approval: existing, Created: false}, nil
	}
	return storage.CreateApprovalResult{}, err
}

func updateApprovalParams(scope storage.Scope, value approvals.Approval, expectedRevision uint64) (dbsqlc.UpdateApprovalParams, error) {
	operationVersion, err := dbOptionalVersion(value.OperationVersion())
	if err != nil {
		return dbsqlc.UpdateApprovalParams{}, err
	}
	revision, err := dbVersion(value.LifecycleRevision())
	if err != nil {
		return dbsqlc.UpdateApprovalParams{}, err
	}
	expected, err := dbVersion(expectedRevision)
	if err != nil {
		return dbsqlc.UpdateApprovalParams{}, err
	}
	return dbsqlc.UpdateApprovalParams{TenantID: scope.TenantID(), ApprovalID: value.ApprovalID(), Status: string(value.Status()), Decision: string(value.Decision()), DecidedAt: dbTime(value.DecidedAt()), ConsumedAt: dbTime(value.ConsumedAt()), OperationKey: dbOptionalString(value.OperationKey()), OperationVersion: operationVersion, LifecycleVersion: revision, LifecycleVersion_2: expected, ActorID: scope.ActorID()}, nil
}
func (s *Store) ConsumeApprovalAndCreateExecution(ctx context.Context, scope storage.Scope, consumed approvals.Approval, expectedRevision uint64, value execution.Execution, record audit.Record) (approvals.Approval, storage.CreateExecutionResult, error) {
	if err := scope.Validate(); err != nil {
		return approvals.Approval{}, storage.CreateExecutionResult{}, err
	}
	if err := consumed.Validate(); err != nil {
		return approvals.Approval{}, storage.CreateExecutionResult{}, err
	}
	if err := value.Validate(); err != nil {
		return approvals.Approval{}, storage.CreateExecutionResult{}, err
	}
	if consumed.UserID() != scope.ActorID() {
		return approvals.Approval{}, storage.CreateExecutionResult{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Approval owner does not match the trusted request scope.", false, true, true)
	}
	if expectedRevision == ^uint64(0) || consumed.LifecycleRevision() != expectedRevision+1 {
		return approvals.Approval{}, storage.CreateExecutionResult{}, apperrors.New(apperrors.CodeExecutionConflict, "Approval revision must advance exactly once.", false, true, true)
	}
	approvalParams, err := updateApprovalParams(scope, consumed, expectedRevision)
	if err != nil {
		return approvals.Approval{}, storage.CreateExecutionResult{}, err
	}
	requestParams, err := requestCreateParams(scope, value.Request())
	if err != nil {
		return approvals.Approval{}, storage.CreateExecutionResult{}, err
	}
	executionParams, err := executionCreateParams(scope, value)
	if err != nil {
		return approvals.Approval{}, storage.CreateExecutionResult{}, err
	}
	auditValue, err := auditParams(scope, record)
	if err != nil {
		return approvals.Approval{}, storage.CreateExecutionResult{}, err
	}
	var approvalRow dbsqlc.Approval
	err = s.withTx(ctx, func(txctx context.Context, q *dbsqlc.Queries) error {
		var err error
		approvalRow, err = q.UpdateApproval(txctx, approvalParams)
		if err != nil {
			return err
		}
		if _, err = q.CreateExecutionRequest(txctx, requestParams); err != nil {
			return err
		}
		if _, err = q.CreateExecution(txctx, executionParams); err != nil {
			return err
		}
		return q.AppendAudit(txctx, auditValue)
	})
	if err == nil {
		restored, err := approvalFromRow(approvalRow)
		return restored, storage.CreateExecutionResult{Execution: value, Created: true}, err
	}
	existing, findErr := s.FindExecutionByOperationKey(ctx, scope, value.Request().OperationKey(), value.Request().OperationVersion())
	if findErr == nil && equalExecutionRequest(existing.Request(), value.Request()) {
		approval, approvalErr := s.FindApprovalByID(ctx, scope, consumed.ApprovalID())
		if approvalErr != nil {
			return approvals.Approval{}, storage.CreateExecutionResult{}, approvalErr
		}
		return approval, storage.CreateExecutionResult{Execution: existing, Created: false}, nil
	}
	if findErr == nil {
		return approvals.Approval{}, storage.CreateExecutionResult{}, apperrors.New(apperrors.CodeExecutionConflict, "Operation already has a different execution request.", false, true, true)
	}
	return approvals.Approval{}, storage.CreateExecutionResult{}, err
}

func executionUpdateParams(scope storage.Scope, value execution.Execution, expected uint64) (dbsqlc.UpdateExecutionParams, error) {
	revision, err := dbVersion(value.Revision())
	if err != nil {
		return dbsqlc.UpdateExecutionParams{}, err
	}
	expectedRevision, err := dbVersion(expected)
	if err != nil {
		return dbsqlc.UpdateExecutionParams{}, err
	}
	failure, _ := value.Failure()
	recovery, _ := value.Recovery()
	return dbsqlc.UpdateExecutionParams{TenantID: scope.TenantID(), ExecutionID: value.ExecutionID(), Status: string(value.Status()), Revision: revision, UpdatedAt: dbTime(value.UpdatedAt()), FailureCode: failure.Code, FailureEligibility: string(failure.Eligibility), FailureRecoveryTarget: string(failure.RecoveryTarget), FailedFromStatus: string(value.FailedFrom()), RecoveryReasonCode: recovery.ReasonCode, RecoveryFromStatus: string(recovery.FromStatus), RecoveryTargetStatus: string(recovery.Target), Revision_2: expectedRevision, ActorID: scope.ActorID()}, nil
}
func (s *Store) UpdateExecutionWithAudit(ctx context.Context, scope storage.Scope, value execution.Execution, expected uint64, record audit.Record) (execution.Execution, error) {
	if err := scope.Validate(); err != nil {
		return execution.Execution{}, err
	}
	if err := value.Validate(); err != nil {
		return execution.Execution{}, err
	}
	if expected == ^uint64(0) || value.Revision() != expected+1 {
		return execution.Execution{}, apperrors.New(apperrors.CodeExecutionConflict, "Execution revision must advance exactly once.", false, true, true)
	}
	update, err := executionUpdateParams(scope, value, expected)
	if err != nil {
		return execution.Execution{}, err
	}
	auditValue, err := auditParams(scope, record)
	if err != nil {
		return execution.Execution{}, err
	}
	err = s.withTx(ctx, func(txctx context.Context, q *dbsqlc.Queries) error {
		if _, err := q.UpdateExecution(txctx, update); err != nil {
			return err
		}
		return q.AppendAudit(txctx, auditValue)
	})
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return execution.Execution{}, apperrors.New(apperrors.CodeExecutionConflict, "Execution changed concurrently.", false, true, true)
		}
		return execution.Execution{}, err
	}
	return value, nil
}
func (s *Store) AppendEvidenceAndVerify(ctx context.Context, scope storage.Scope, evidence execution.Result, verified execution.Execution, expected uint64, record audit.Record) (execution.Execution, error) {
	if err := scope.Validate(); err != nil {
		return execution.Execution{}, err
	}
	if err := evidence.Validate(); err != nil {
		return execution.Execution{}, err
	}
	if err := verified.Validate(); err != nil {
		return execution.Execution{}, err
	}
	if verified.Status() != execution.StatusVerified || evidence.Status() != execution.StatusVerified || evidence.ExecutionID() != verified.ExecutionID() || expected == ^uint64(0) || verified.Revision() != expected+1 || evidence.ExecutionVersion() != verified.Revision() {
		return execution.Execution{}, apperrors.New(apperrors.CodeExecutionInvalid, "Verification evidence does not match the verified execution.", false, true, true)
	}
	version, err := dbVersion(evidence.ExecutionVersion())
	if err != nil {
		return execution.Execution{}, err
	}
	update, err := executionUpdateParams(scope, verified, expected)
	if err != nil {
		return execution.Execution{}, err
	}
	auditValue, err := auditParams(scope, record)
	if err != nil {
		return execution.Execution{}, err
	}
	evidenceParams := dbsqlc.AppendVerificationEvidenceParams{TenantID: scope.TenantID(), ExecutionID: evidence.ExecutionID(), ExecutionVersion: version, Status: string(evidence.Status()), AdapterReference: evidence.AdapterReference(), ErrorCode: evidence.ErrorCode(), ObservedAt: dbTime(evidence.ObservedAt()), ActorID: scope.ActorID()}
	err = s.withTx(ctx, func(txctx context.Context, q *dbsqlc.Queries) error {
		if _, err := q.UpdateExecution(txctx, update); err != nil {
			return err
		}
		inserted, err := q.AppendVerificationEvidence(txctx, evidenceParams)
		if err != nil {
			return err
		}
		if inserted != 1 {
			return pgx.ErrNoRows
		}
		return q.AppendAudit(txctx, auditValue)
	})
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return execution.Execution{}, apperrors.New(apperrors.CodeExecutionConflict, "Execution changed concurrently.", false, true, true)
		}
		return execution.Execution{}, err
	}
	return verified, nil
}

var _ storage.AtomicRepository = (*Store)(nil)
