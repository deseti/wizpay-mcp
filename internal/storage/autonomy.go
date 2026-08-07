package storage

import (
	"context"
	"time"

	"github.com/deseti/wizpay-mcp/internal/autonomy"
)

// AutonomyRepository is the durable boundary used by the scheduler. A
// PostgreSQL implementation must scope every method by Scope, claim with a
// lease/fencing update, and enforce the occurrence/reservation uniqueness
// constraints from migration 000004.
type AutonomyRepository interface {
	ListAutonomySchedules(context.Context, Scope) ([]autonomy.Schedule, error)
	SaveAutonomySchedule(context.Context, Scope, autonomy.Schedule) error
	LoadAutonomySchedule(context.Context, Scope, string, uint64) (autonomy.Schedule, error)
	SaveAutonomyOccurrence(context.Context, Scope, autonomy.Occurrence) error
	LoadAutonomyOccurrence(context.Context, Scope, string) (autonomy.Occurrence, error)
	ClaimAutonomyDue(context.Context, Scope, time.Time, string, time.Duration) (autonomy.Occurrence, bool, error)
	ClaimNextAutonomyDue(context.Context, string, time.Time, time.Duration) (Scope, autonomy.Occurrence, bool, error)
	BlockAutonomyOccurrence(context.Context, Scope, autonomy.Occurrence, autonomy.ReasonCode) error
	BindAutonomyApproval(context.Context, Scope, autonomy.Occurrence, string) error
	CheckAutonomyDispatchFence(context.Context, Scope, autonomy.Occurrence, string, uint64, time.Time) (autonomy.Occurrence, bool, error)
	ReserveAutonomySpend(context.Context, Scope, autonomy.Grant, string, string, time.Time) (bool, error)
	ReleaseAutonomySpend(context.Context, Scope, string) error
	LoadAutonomyEmergencyStop(context.Context, Scope) (autonomy.EmergencyStop, error)
	SetAutonomyEmergencyStop(context.Context, Scope, autonomy.EmergencyStop) error
	SaveAutonomyGrant(context.Context, Scope, autonomy.Grant) error
	LoadAutonomyGrant(context.Context, Scope, string, uint64) (autonomy.Grant, error)
	SaveAutonomyDelegation(context.Context, Scope, autonomy.Delegation) error
	LoadAutonomyDelegation(context.Context, Scope, string, uint64) (autonomy.Delegation, error)
}
