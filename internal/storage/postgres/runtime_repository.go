package postgres

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deseti/wizpay-mcp/internal/audit"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/storage/postgres/dbsqlc"
)

func validateClaimInput(workerID string, now time.Time, leaseDuration time.Duration) error {
	if workerID == "" || now.IsZero() || leaseDuration <= 0 || leaseDuration > time.Hour {
		return apperrors.New(apperrors.CodeValidationError, "Execution work claim is invalid.", false, true, true)
	}
	return nil
}

func executionClaim(scope storage.Scope, value dbsqlc.ExecutionRuntimeWork, executionValue storage.CreateExecutionResult) (storage.ExecutionClaim, error) {
	token, err := domainVersion(value.FencingToken)
	if err != nil {
		return storage.ExecutionClaim{}, err
	}
	return storage.ExecutionClaim{
		Scope: scope, Execution: executionValue.Execution, LeaseOwner: value.LeaseOwner,
		FencingToken: token, LeaseExpiresAt: domainTime(value.LeaseExpiresAt), SubmissionStarted: value.SubmissionStarted,
	}, nil
}

func (s *Store) ClaimExecutionWork(ctx context.Context, scope storage.Scope, executionID, workerID string, now time.Time, leaseDuration time.Duration) (storage.ExecutionClaim, bool, error) {
	if err := scope.Validate(); err != nil {
		return storage.ExecutionClaim{}, false, err
	}
	if executionID == "" {
		return storage.ExecutionClaim{}, false, apperrors.New(apperrors.CodeValidationError, "Execution identity is required.", false, true, true)
	}
	if err := validateClaimInput(workerID, now, leaseDuration); err != nil {
		return storage.ExecutionClaim{}, false, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return storage.ExecutionClaim{}, false, err
	}
	defer cancel()
	row, err := s.queries.ClaimExecutionWork(bounded, dbsqlc.ClaimExecutionWorkParams{LeaseOwner: workerID, LeaseDurationMicroseconds: leaseDuration.Microseconds(), TenantID: scope.TenantID(), ExecutionID: executionID, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return storage.ExecutionClaim{}, false, nil
	}
	if err != nil {
		return storage.ExecutionClaim{}, false, mapDatabaseError(err)
	}
	value, err := s.FindExecutionByID(ctx, scope, executionID)
	if err != nil {
		return storage.ExecutionClaim{}, false, err
	}
	rows, err := s.queries.ValidateExecutionClaim(bounded, dbsqlc.ValidateExecutionClaimParams{TenantID: scope.TenantID(), ExecutionID: executionID, LeaseOwner: workerID, FencingToken: row.FencingToken})
	if err != nil {
		return storage.ExecutionClaim{}, false, mapDatabaseError(err)
	}
	if rows != 1 {
		return storage.ExecutionClaim{}, false, apperrors.New(apperrors.CodeExecutionConflict, "Execution work claim changed concurrently.", true, false, false)
	}
	claim, err := executionClaim(scope, row, storage.CreateExecutionResult{Execution: value})
	return claim, err == nil, err
}

func (s *Store) ClaimNextExecutionWork(ctx context.Context, workerID string, now time.Time, leaseDuration time.Duration) (storage.ExecutionClaim, bool, error) {
	if err := validateClaimInput(workerID, now, leaseDuration); err != nil {
		return storage.ExecutionClaim{}, false, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return storage.ExecutionClaim{}, false, err
	}
	defer cancel()
	row, err := s.queries.ClaimNextExecutionWork(bounded, dbsqlc.ClaimNextExecutionWorkParams{LeaseOwner: workerID, LeaseDurationMicroseconds: leaseDuration.Microseconds()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return storage.ExecutionClaim{}, false, nil
	}
	if err != nil {
		return storage.ExecutionClaim{}, false, mapDatabaseError(err)
	}
	owner, err := s.queries.FindExecutionOwner(bounded, dbsqlc.FindExecutionOwnerParams{TenantID: row.TenantID, ExecutionID: row.ExecutionID})
	if err != nil {
		return storage.ExecutionClaim{}, false, mapDatabaseError(err)
	}
	scope, err := storage.NewScope(owner.TenantID, owner.UserID, fmt.Sprintf("runtime:%s:%d", workerID, row.FencingToken), "")
	if err != nil {
		return storage.ExecutionClaim{}, false, err
	}
	value, err := s.FindExecutionByID(ctx, scope, row.ExecutionID)
	if err != nil {
		return storage.ExecutionClaim{}, false, err
	}
	rows, err := s.queries.ValidateExecutionClaim(bounded, dbsqlc.ValidateExecutionClaimParams{TenantID: row.TenantID, ExecutionID: row.ExecutionID, LeaseOwner: workerID, FencingToken: row.FencingToken})
	if err != nil {
		return storage.ExecutionClaim{}, false, mapDatabaseError(err)
	}
	if rows != 1 {
		return storage.ExecutionClaim{}, false, apperrors.New(apperrors.CodeExecutionConflict, "Execution work claim changed concurrently.", true, false, false)
	}
	claim, err := executionClaim(scope, row, storage.CreateExecutionResult{Execution: value})
	return claim, err == nil, err
}

func (s *Store) MarkSubmissionStarted(ctx context.Context, claim storage.ExecutionClaim, now time.Time) (storage.ExecutionClaim, bool, error) {
	token, err := dbVersion(claim.FencingToken)
	if err != nil {
		return storage.ExecutionClaim{}, false, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return storage.ExecutionClaim{}, false, err
	}
	defer cancel()
	row, err := s.queries.MarkSubmissionStarted(bounded, dbsqlc.MarkSubmissionStartedParams{TenantID: claim.Scope.TenantID(), ExecutionID: claim.Execution.ExecutionID(), LeaseOwner: claim.LeaseOwner, FencingToken: token})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return claim, false, nil
	}
	if err != nil {
		return storage.ExecutionClaim{}, false, mapDatabaseError(err)
	}
	updated, err := executionClaim(claim.Scope, row, storage.CreateExecutionResult{Execution: claim.Execution})
	return updated, err == nil, err
}

func (s *Store) ResetSubmissionStarted(ctx context.Context, claim storage.ExecutionClaim, now time.Time) (storage.ExecutionClaim, bool, error) {
	token, err := validateClaimParams(claim, now)
	if err != nil {
		return storage.ExecutionClaim{}, false, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return storage.ExecutionClaim{}, false, err
	}
	defer cancel()
	rows, err := s.queries.ResetSubmissionStarted(bounded, dbsqlc.ResetSubmissionStartedParams{TenantID: claim.Scope.TenantID(), ExecutionID: claim.Execution.ExecutionID(), LeaseOwner: claim.LeaseOwner, FencingToken: token})
	if err != nil {
		return storage.ExecutionClaim{}, false, mapDatabaseError(err)
	}
	if rows != 1 {
		return claim, false, nil
	}
	claim.SubmissionStarted = false
	return claim, true, nil
}

func validateClaimParams(claim storage.ExecutionClaim, now time.Time) (int64, error) {
	if err := claim.Scope.Validate(); err != nil {
		return 0, err
	}
	if claim.LeaseOwner == "" || now.IsZero() {
		return 0, apperrors.New(apperrors.CodeValidationError, "Execution work claim is invalid.", false, true, true)
	}
	return dbVersion(claim.FencingToken)
}

func validateRuntimeAudit(claim storage.ExecutionClaim, next execution.Execution, record audit.Record) error {
	current := claim.Execution
	if current.ExecutionID() != next.ExecutionID() || record.ResourceType != "execution" || record.ResourceID != next.ExecutionID() ||
		record.PreviousState != string(current.Status()) || record.NewState != string(next.Status()) ||
		record.Event.ExecutionID != next.ExecutionID() || record.Event.ExecutionRevision != next.Revision() ||
		record.Event.ExecutionStatus != string(next.Status()) {
		return apperrors.New(apperrors.CodeExecutionInvalid, "Execution audit does not match the claimed transition.", false, true, true)
	}
	return nil
}

func (s *Store) UpdateClaimedExecution(ctx context.Context, claim storage.ExecutionClaim, value execution.Execution, expected uint64, record audit.Record, now time.Time) (execution.Execution, error) {
	token, err := validateClaimParams(claim, now)
	if err != nil {
		return execution.Execution{}, err
	}
	if err := value.Validate(); err != nil {
		return execution.Execution{}, err
	}
	if err := validateRuntimeAudit(claim, value, record); err != nil {
		return execution.Execution{}, err
	}
	update, err := executionUpdateParams(claim.Scope, value, expected)
	if err != nil {
		return execution.Execution{}, err
	}
	auditValue, err := auditParams(claim.Scope, record)
	if err != nil {
		return execution.Execution{}, err
	}
	err = s.withTx(ctx, func(txctx context.Context, q *dbsqlc.Queries) error {
		rows, err := q.ValidateExecutionClaim(txctx, dbsqlc.ValidateExecutionClaimParams{TenantID: claim.Scope.TenantID(), ExecutionID: claim.Execution.ExecutionID(), LeaseOwner: claim.LeaseOwner, FencingToken: token})
		if err != nil {
			return err
		}
		if rows != 1 {
			return pgx.ErrNoRows
		}
		if _, err = q.UpdateExecution(txctx, update); err != nil {
			return err
		}
		return q.AppendAudit(txctx, auditValue)
	})
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return execution.Execution{}, apperrors.New(apperrors.CodeExecutionConflict, "Execution work claim or revision changed concurrently.", true, false, false)
		}
		return execution.Execution{}, err
	}
	return value, nil
}

func (s *Store) PersistClaimedObservation(ctx context.Context, claim storage.ExecutionClaim, evidence execution.Result, value execution.Execution, expected uint64, record audit.Record, now time.Time) (execution.Execution, error) {
	token, err := validateClaimParams(claim, now)
	if err != nil {
		return execution.Execution{}, err
	}
	if err := evidence.Validate(); err != nil {
		return execution.Execution{}, err
	}
	if err := validateRuntimeAudit(claim, value, record); err != nil {
		return execution.Execution{}, err
	}
	if evidence.ExecutionID() != value.ExecutionID() || evidence.ExecutionVersion() != value.Revision() || value.Revision() != expected+1 {
		return execution.Execution{}, apperrors.New(apperrors.CodeExecutionInvalid, "Execution observation does not match the execution revision.", false, true, true)
	}
	version, err := dbVersion(evidence.ExecutionVersion())
	if err != nil {
		return execution.Execution{}, err
	}
	update, err := executionUpdateParams(claim.Scope, value, expected)
	if err != nil {
		return execution.Execution{}, err
	}
	auditValue, err := auditParams(claim.Scope, record)
	if err != nil {
		return execution.Execution{}, err
	}
	evidenceParams := dbsqlc.AppendVerificationEvidenceParams{TenantID: claim.Scope.TenantID(), ExecutionID: evidence.ExecutionID(), ExecutionVersion: version, Status: string(evidence.Status()), AdapterReference: evidence.AdapterReference(), ObservedAt: dbTime(evidence.ObservedAt()), ActorID: claim.Scope.ActorID()}
	err = s.withTx(ctx, func(txctx context.Context, q *dbsqlc.Queries) error {
		rows, err := q.ValidateExecutionClaim(txctx, dbsqlc.ValidateExecutionClaimParams{TenantID: claim.Scope.TenantID(), ExecutionID: claim.Execution.ExecutionID(), LeaseOwner: claim.LeaseOwner, FencingToken: token})
		if err != nil {
			return err
		}
		if rows != 1 {
			return pgx.ErrNoRows
		}
		if _, err = q.UpdateExecution(txctx, update); err != nil {
			return err
		}
		inserted, err := q.AppendVerificationEvidence(txctx, evidenceParams)
		if err != nil || inserted != 1 {
			if err != nil {
				return err
			}
			return pgx.ErrNoRows
		}
		return q.AppendAudit(txctx, auditValue)
	})
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return execution.Execution{}, apperrors.New(apperrors.CodeExecutionConflict, "Execution work claim, revision, or observation changed concurrently.", true, false, false)
		}
		return execution.Execution{}, err
	}
	return value, nil
}

func (s *Store) PersistClaimedEvidence(ctx context.Context, claim storage.ExecutionClaim, evidence execution.Result, value execution.Execution, expected uint64, record audit.Record, now time.Time) (execution.Execution, error) {
	token, err := validateClaimParams(claim, now)
	if err != nil {
		return execution.Execution{}, err
	}
	if err := evidence.Validate(); err != nil {
		return execution.Execution{}, err
	}
	if err := validateRuntimeAudit(claim, value, record); err != nil {
		return execution.Execution{}, err
	}
	if value.Status() != execution.StatusVerified || evidence.Status() != execution.StatusVerified || evidence.ExecutionID() != value.ExecutionID() || value.Revision() != expected+1 || evidence.ExecutionVersion() != value.Revision() {
		return execution.Execution{}, apperrors.New(apperrors.CodeExecutionInvalid, "Verification evidence does not match the verified execution.", false, true, true)
	}
	version, err := dbVersion(evidence.ExecutionVersion())
	if err != nil {
		return execution.Execution{}, err
	}
	update, err := executionUpdateParams(claim.Scope, value, expected)
	if err != nil {
		return execution.Execution{}, err
	}
	auditValue, err := auditParams(claim.Scope, record)
	if err != nil {
		return execution.Execution{}, err
	}
	evidenceParams := dbsqlc.AppendVerificationEvidenceParams{TenantID: claim.Scope.TenantID(), ExecutionID: evidence.ExecutionID(), ExecutionVersion: version, Status: string(evidence.Status()), AdapterReference: evidence.AdapterReference(), ErrorCode: evidence.ErrorCode(), ObservedAt: dbTime(evidence.ObservedAt()), ActorID: claim.Scope.ActorID()}
	err = s.withTx(ctx, func(txctx context.Context, q *dbsqlc.Queries) error {
		rows, err := q.ValidateExecutionClaim(txctx, dbsqlc.ValidateExecutionClaimParams{TenantID: claim.Scope.TenantID(), ExecutionID: claim.Execution.ExecutionID(), LeaseOwner: claim.LeaseOwner, FencingToken: token})
		if err != nil {
			return err
		}
		if rows != 1 {
			return pgx.ErrNoRows
		}
		if _, err = q.UpdateExecution(txctx, update); err != nil {
			return err
		}
		inserted, err := q.AppendVerificationEvidence(txctx, evidenceParams)
		if err != nil || inserted != 1 {
			if err != nil {
				return err
			}
			return pgx.ErrNoRows
		}
		return q.AppendAudit(txctx, auditValue)
	})
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return execution.Execution{}, apperrors.New(apperrors.CodeExecutionConflict, "Execution work claim, revision, or evidence changed concurrently.", true, false, false)
		}
		return execution.Execution{}, err
	}
	return value, nil
}

func (s *Store) ReleaseExecutionWork(ctx context.Context, claim storage.ExecutionClaim, nextRunAt time.Time) (bool, error) {
	if err := claim.Scope.Validate(); err != nil {
		return false, err
	}
	if nextRunAt.IsZero() {
		return false, apperrors.New(apperrors.CodeValidationError, "Next execution run time is required.", false, true, true)
	}
	token, err := dbVersion(claim.FencingToken)
	if err != nil {
		return false, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return false, err
	}
	defer cancel()
	rows, err := s.queries.ReleaseExecutionWork(bounded, dbsqlc.ReleaseExecutionWorkParams{TenantID: claim.Scope.TenantID(), ExecutionID: claim.Execution.ExecutionID(), LeaseOwner: claim.LeaseOwner, FencingToken: token, NextRunAt: dbTime(nextRunAt)})
	if err != nil {
		return false, mapDatabaseError(err)
	}
	if rows != 1 {
		return false, apperrors.New(apperrors.CodeExecutionConflict, "Execution work claim changed concurrently.", true, false, false)
	}
	return true, nil
}

var _ storage.ExecutionRuntimeRepository = (*Store)(nil)
