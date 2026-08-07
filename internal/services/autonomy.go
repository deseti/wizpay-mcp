package services

import (
	"context"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/audit"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/autonomy"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

// PersistedAutonomyService is the authenticated application boundary for
// schedule controls. It derives tenant and actor from TrustedRequest; caller
// supplied principal fields are never used as authority.
type PersistedAutonomyService struct {
	Repository storage.AutonomyRepository
	Authorizer auth.Authorizer
	Audit      storage.AuditRepository
	Wallets    storage.WalletBindingRepository
	Now        func() time.Time
	Enabled    bool
}

func (s *PersistedAutonomyService) scope(ctx context.Context, permission auth.Permission) (storage.Scope, error) {
	if s == nil || s.Repository == nil || s.Authorizer == nil || s.Now == nil {
		return storage.Scope{}, fmt.Errorf("autonomy service is not configured")
	}
	request, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		return storage.Scope{}, err
	}
	if err := s.Authorizer.Authorize(ctx, auth.AuthorizationInput{Request: request, Permission: permission}); err != nil {
		return storage.Scope{}, err
	}
	return requestauth.StorageScopeFromContext(ctx)
}
func (s *PersistedAutonomyService) ListSchedules(ctx context.Context) ([]autonomy.Schedule, error) {
	scope, err := s.scope(ctx, auth.PermissionAutonomyRead)
	if err != nil {
		return nil, err
	}
	return s.Repository.ListAutonomySchedules(ctx, scope)
}
func (s *PersistedAutonomyService) GetSchedule(ctx context.Context, id string, v uint64) (autonomy.Schedule, error) {
	scope, err := s.scope(ctx, auth.PermissionAutonomyRead)
	if err != nil {
		return autonomy.Schedule{}, err
	}
	return s.Repository.LoadAutonomySchedule(ctx, scope, id, v)
}
func (s *PersistedAutonomyService) CreateSchedule(ctx context.Context, value autonomy.Schedule) (autonomy.Schedule, error) {
	scope, err := s.scope(ctx, auth.PermissionAutonomyControl)
	if err != nil {
		return autonomy.Schedule{}, err
	}
	request, _ := auth.TrustedRequestFromContext(ctx)
	value.Principal.TenantID = scope.TenantID()
	value.Principal.UserID = request.Principal().ActorID()
	if value.Principal.AgentID == "" {
		return autonomy.Schedule{}, fmt.Errorf("acting agent is required")
	}
	if err := value.Validate(); err != nil {
		return autonomy.Schedule{}, err
	}
	grant, err := s.Repository.LoadAutonomyGrant(ctx, scope, value.GrantID, value.GrantVersion)
	if err != nil {
		return autonomy.Schedule{}, err
	}
	delegation, err := s.Repository.LoadAutonomyDelegation(ctx, scope, value.DelegationID, value.DelegationVersion)
	if err != nil {
		return autonomy.Schedule{}, err
	}
	if grant.PrincipalUserID != value.Principal.UserID || grant.WalletBindingID != value.WalletBindingID || grant.Intent != value.Spec.Intent || grant.ScheduleID != "" && grant.ScheduleID != value.ID || !grant.Active(s.Now().UTC()) {
		return autonomy.Schedule{}, fmt.Errorf("grant is not authorized for schedule")
	}
	if delegation.PrincipalUserID != value.Principal.UserID || delegation.AgentID != value.Principal.AgentID || delegation.Validate(s.Now().UTC(), value.Spec.Intent) != nil {
		return autonomy.Schedule{}, fmt.Errorf("delegation is not authorized for schedule")
	}
	if s.Wallets == nil {
		return autonomy.Schedule{}, fmt.Errorf("wallet binding repository is required")
	}
	binding, err := s.Wallets.FindBindingByID(ctx, scope, value.WalletBindingID)
	if err != nil {
		return autonomy.Schedule{}, err
	}
	if binding.Version() != value.WalletBindingVersion || binding.EnsureAuthorizable(value.Principal.UserID) != nil {
		return autonomy.Schedule{}, fmt.Errorf("wallet binding is not authorized for schedule")
	}
	if value.Digest != value.ComputeDigest() {
		return autonomy.Schedule{}, fmt.Errorf("schedule digest mismatch")
	}
	if err := s.appendAudit(ctx, scope, audit.EventAutonomyScheduleCreated, value.ID, "", string(value.Status)); err != nil {
		return autonomy.Schedule{}, err
	}
	if err := s.Repository.SaveAutonomySchedule(ctx, scope, value); err != nil {
		return autonomy.Schedule{}, err
	}
	return value, nil
}
func (s *PersistedAutonomyService) SetScheduleStatus(ctx context.Context, id string, v uint64, status autonomy.ScheduleStatus) (autonomy.Schedule, error) {
	scope, err := s.scope(ctx, auth.PermissionAutonomyControl)
	if err != nil {
		return autonomy.Schedule{}, err
	}
	value, err := s.Repository.LoadAutonomySchedule(ctx, scope, id, v)
	if err != nil {
		return autonomy.Schedule{}, err
	}
	if status != autonomy.ScheduleActive && status != autonomy.SchedulePaused && status != autonomy.ScheduleRevoked {
		return autonomy.Schedule{}, fmt.Errorf("invalid schedule status")
	}
	if !autonomy.ValidScheduleTransition(value.Status, status) {
		return autonomy.Schedule{}, fmt.Errorf("invalid schedule status transition")
	}
	value.Version++
	value.Status = status
	value.UpdatedAt = s.Now().UTC()
	value.Digest = value.ComputeDigest()
	if err := s.appendAudit(ctx, scope, audit.EventAutonomyScheduleVersioned, value.ID, "", string(value.Status)); err != nil {
		return autonomy.Schedule{}, err
	}
	if err := s.Repository.SaveAutonomySchedule(ctx, scope, value); err != nil {
		return autonomy.Schedule{}, err
	}
	return value, nil
}
func (s *PersistedAutonomyService) SetEmergencyStop(ctx context.Context, value autonomy.EmergencyStop) (autonomy.EmergencyStop, error) {
	scope, err := s.scope(ctx, auth.PermissionAutonomyControl)
	if err != nil {
		return autonomy.EmergencyStop{}, err
	}
	request, _ := auth.TrustedRequestFromContext(ctx)
	value.Actor = request.Principal().ActorID()
	value.Scope = "TENANT"
	value.ChangedAt = s.Now().UTC()
	if err := value.Validate(); err != nil {
		return autonomy.EmergencyStop{}, err
	}
	if err := s.appendAudit(ctx, scope, audit.EventAutonomyEmergencyStop, scope.TenantID(), "", fmt.Sprint(value.Active)); err != nil {
		return autonomy.EmergencyStop{}, err
	}
	if err := s.Repository.SetAutonomyEmergencyStop(ctx, scope, value); err != nil {
		return autonomy.EmergencyStop{}, err
	}
	return value, nil
}
func (s *PersistedAutonomyService) SimulateOccurrence(ctx context.Context, id string, version uint64, at time.Time) (autonomy.Decision, error) {
	scope, err := s.scope(ctx, auth.PermissionAutonomyRead)
	if err != nil {
		return autonomy.Decision{}, err
	}
	schedule, err := s.Repository.LoadAutonomySchedule(ctx, scope, id, version)
	if err != nil {
		return autonomy.Decision{}, err
	}
	occurrence := autonomy.NewOccurrence(schedule, at)
	decision := autonomy.Decision{ScheduleID: schedule.ID, OccurrenceID: occurrence.ID, GrantID: schedule.GrantID, Eligible: false}
	if !s.Enabled {
		decision.Reason = autonomy.ReasonRuntimeDisabled
		return decision, nil
	}
	if schedule.Status != autonomy.ScheduleActive {
		decision.Reason = autonomy.ReasonSchedulePaused
		return decision, nil
	}
	// The simulation intentionally stops at capability/provider availability
	// until a typed Payroll/Swap planner is assembled. It performs no reserve,
	// approval, intent, execution, or provider operation.
	decision.Reason = autonomy.ReasonProviderUnavailable
	return decision, nil
}

func (s *PersistedAutonomyService) appendAudit(ctx context.Context, scope storage.Scope, eventType audit.EventType, resourceID, previous, next string) error {
	if s.Audit == nil {
		return fmt.Errorf("durable autonomy audit repository is required")
	}
	return s.Audit.AppendAudit(ctx, scope, audit.Record{Event: audit.Event{EventID: scope.RequestID() + "/" + resourceID + "/" + string(eventType), Type: eventType, OccurredAt: s.Now().UTC()}, ActorType: "user", ActorID: scope.ActorID(), RequestID: scope.RequestID(), TraceID: scope.TraceID(), ResourceType: "autonomy", ResourceID: resourceID, PreviousState: previous, NewState: next, SourceComponent: "autonomy.service"})
}
