package postgres

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/audit"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/storage/postgres/dbsqlc"
)

func auditParams(scope storage.Scope, record audit.Record) (dbsqlc.AppendAuditParams, error) {
	if err := scope.Validate(); err != nil {
		return dbsqlc.AppendAuditParams{}, err
	}
	if err := record.Validate(); err != nil {
		return dbsqlc.AppendAuditParams{}, err
	}
	if record.ActorID != scope.ActorID() || record.RequestID != scope.RequestID() || record.TraceID != scope.TraceID() {
		return dbsqlc.AppendAuditParams{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Audit attribution does not match the trusted request scope.", false, true, true)
	}
	event := record.Event
	return dbsqlc.AppendAuditParams{TenantID: scope.TenantID(), EventID: event.EventID, EventType: string(event.Type), OccurredAt: dbTime(event.OccurredAt), ActorType: record.ActorType, ActorID: record.ActorID, RequestID: record.RequestID, TraceID: record.TraceID, ResourceType: record.ResourceType, ResourceID: record.ResourceID, PreviousState: record.PreviousState, NewState: record.NewState, SafeReasonCode: record.SafeReasonCode, SourceComponent: record.SourceComponent, IntentID: event.IntentID, IntentVersion: int64(event.IntentVersion), IntentDigest: event.IntentDigest, ApprovalID: event.ApprovalID, PolicyID: event.PolicyID, PolicyVersion: int64(event.PolicyVersion), PolicyDecision: event.PolicyDecision, PolicyEvaluationKey: event.PolicyEvaluationKey, ExecutionID: event.ExecutionID, ExecutionRevision: int64(event.ExecutionRevision), ExecutionRequestID: event.ExecutionRequestID, ExecutionRequestKey: event.ExecutionRequestKey, ExecutionStatus: event.ExecutionStatus, RecoveryReasonCode: event.RecoveryReasonCode, WalletBindingID: event.WalletBindingID, WalletBindingVersion: int64(event.WalletBindingVersion), UserID: event.UserID, OperationKey: event.OperationKey, OperationVersion: int64(event.OperationVersion)}, nil
}
func (s *Store) AppendAudit(ctx context.Context, scope storage.Scope, record audit.Record) error {
	params, err := auditParams(scope, record)
	if err != nil {
		return err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	return mapDatabaseError(s.queries.AppendAudit(bounded, params))
}
func (s *Store) FindAuditByResource(ctx context.Context, scope storage.Scope, resourceType, resourceID string) ([]audit.Record, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	rows, err := s.queries.FindAuditByResource(bounded, dbsqlc.FindAuditByResourceParams{TenantID: scope.TenantID(), ResourceType: resourceType, ResourceID: resourceID})
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	result := make([]audit.Record, 0, len(rows))
	for _, row := range rows {
		event := audit.Event{EventID: row.EventID, Type: audit.EventType(row.EventType), OccurredAt: domainTime(row.OccurredAt), IntentID: row.IntentID, IntentVersion: uint64(row.IntentVersion), IntentDigest: row.IntentDigest, ApprovalID: row.ApprovalID, PolicyID: row.PolicyID, PolicyVersion: uint64(row.PolicyVersion), PolicyDecision: row.PolicyDecision, PolicyEvaluationKey: row.PolicyEvaluationKey, ExecutionID: row.ExecutionID, ExecutionRevision: uint64(row.ExecutionRevision), ExecutionRequestID: row.ExecutionRequestID, ExecutionRequestKey: row.ExecutionRequestKey, ExecutionStatus: row.ExecutionStatus, RecoveryReasonCode: row.RecoveryReasonCode, WalletBindingID: row.WalletBindingID, WalletBindingVersion: uint64(row.WalletBindingVersion), UserID: row.UserID, OperationKey: row.OperationKey, OperationVersion: uint64(row.OperationVersion)}
		result = append(result, audit.Record{Event: event, ActorType: row.ActorType, ActorID: row.ActorID, RequestID: row.RequestID, TraceID: row.TraceID, ResourceType: row.ResourceType, ResourceID: row.ResourceID, PreviousState: row.PreviousState, NewState: row.NewState, SafeReasonCode: row.SafeReasonCode, SourceComponent: row.SourceComponent})
	}
	return result, nil
}

var _ storage.AuditRepository = (*Store)(nil)
