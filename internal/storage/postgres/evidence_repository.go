package postgres

import (
	"context"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/storage/postgres/dbsqlc"
)

func (s *Store) AppendVerificationEvidence(ctx context.Context, scope storage.Scope, value execution.Result) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return err
	}
	version, err := dbVersion(value.ExecutionVersion())
	if err != nil {
		return err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	inserted, err := s.queries.AppendVerificationEvidence(bounded, dbsqlc.AppendVerificationEvidenceParams{TenantID: scope.TenantID(), ExecutionID: value.ExecutionID(), ExecutionVersion: version, Status: string(value.Status()), AdapterReference: value.AdapterReference(), ErrorCode: value.ErrorCode(), ObservedAt: dbTime(value.ObservedAt()), ActorID: scope.ActorID()})
	if err != nil {
		return mapDatabaseError(err)
	}
	if inserted != 1 {
		return apperrors.New(apperrors.CodeExecutionConflict, "Verification evidence does not match the current execution revision.", false, true, true)
	}
	return nil
}
func (s *Store) FindVerificationEvidence(ctx context.Context, scope storage.Scope, executionID string) ([]execution.Result, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	rows, err := s.queries.FindVerificationEvidence(bounded, dbsqlc.FindVerificationEvidenceParams{TenantID: scope.TenantID(), ExecutionID: executionID, ActorID: scope.ActorID()})
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	result := make([]execution.Result, 0, len(rows))
	for _, row := range rows {
		version, err := domainVersion(row.ExecutionVersion)
		if err != nil {
			return nil, err
		}
		value, err := execution.NewResult(execution.ResultParams{ExecutionID: row.ExecutionID, ExecutionVersion: version, Status: execution.Status(row.Status), AdapterReference: row.AdapterReference, ErrorCode: row.ErrorCode, ObservedAt: domainTime(row.ObservedAt)})
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

var _ storage.VerificationEvidenceRepository = (*Store)(nil)
