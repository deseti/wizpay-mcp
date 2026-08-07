package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/autonomy"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

// RuntimeStore binds the narrow autonomy runtime contract to one authenticated
// durable scope. It is the only adapter allowed to hide Scope from runtime
// calls; every operation remains delegated to the scoped repository.
type RuntimeStore struct {
	repository storage.AutonomyRepository
	scope      storage.Scope
}

func NewRuntimeStore(repository storage.AutonomyRepository, scope storage.Scope) (*RuntimeStore, error) {
	if repository == nil {
		return nil, fmt.Errorf("autonomy repository is required")
	}
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	return &RuntimeStore{repository: repository, scope: scope}, nil
}

func (s *RuntimeStore) SaveSchedule(ctx context.Context, value autonomy.Schedule) error {
	return s.repository.SaveAutonomySchedule(ctx, s.scope, value)
}
func (s *RuntimeStore) GetSchedule(ctx context.Context, id string, version uint64) (autonomy.Schedule, error) {
	return s.repository.LoadAutonomySchedule(ctx, s.scope, id, version)
}
func (s *RuntimeStore) SaveOccurrence(ctx context.Context, value autonomy.Occurrence) error {
	return s.repository.SaveAutonomyOccurrence(ctx, s.scope, value)
}
func (s *RuntimeStore) GetOccurrence(ctx context.Context, id string) (autonomy.Occurrence, error) {
	return s.repository.LoadAutonomyOccurrence(ctx, s.scope, id)
}
func (s *RuntimeStore) ClaimDue(ctx context.Context, at time.Time, worker string, lease time.Duration) (autonomy.Occurrence, bool, error) {
	return s.repository.ClaimAutonomyDue(ctx, s.scope, at, worker, lease)
}
func (s *RuntimeStore) Reserve(ctx context.Context, occurrenceID, grantID string, grantVersion uint64, amount string, at time.Time) (bool, error) {
	grant, err := s.repository.LoadAutonomyGrant(ctx, s.scope, grantID, grantVersion)
	if err != nil {
		return false, err
	}
	return s.repository.ReserveAutonomySpend(ctx, s.scope, grant, occurrenceID, amount, at)
}
func (s *RuntimeStore) ReleaseReservation(ctx context.Context, occurrenceID string) error {
	return s.repository.ReleaseAutonomySpend(ctx, s.scope, occurrenceID)
}
func (s *RuntimeStore) Emergency(ctx context.Context) (autonomy.EmergencyStop, error) {
	return s.repository.LoadAutonomyEmergencyStop(ctx, s.scope)
}
func (s *RuntimeStore) SetEmergency(ctx context.Context, value autonomy.EmergencyStop) error {
	return s.repository.SetAutonomyEmergencyStop(ctx, s.scope, value)
}
func (s *RuntimeStore) CheckDispatchFence(ctx context.Context, occurrenceID, worker string, fence uint64, at time.Time) (autonomy.Occurrence, bool, error) {
	return s.repository.CheckAutonomyDispatchFence(ctx, s.scope, autonomy.Occurrence{ID: occurrenceID}, worker, fence, at)
}

var _ autonomy.Store = (*RuntimeStore)(nil)
