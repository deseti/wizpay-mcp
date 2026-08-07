package autonomy

import (
	"context"
	"fmt"
	"time"
)

type Scheduler struct {
	Store     Store
	Now       func() time.Time
	WorkerID  string
	Lease     time.Duration
	MaxMissed int
}

func (s Scheduler) Validate() error {
	if s.Store == nil || s.Now == nil || s.WorkerID == "" || s.Lease <= 0 {
		return fmt.Errorf("scheduler configuration is invalid")
	}
	if s.MaxMissed < 1 || s.MaxMissed > 100 {
		return fmt.Errorf("missed-run bound is invalid")
	}
	return nil
}

// Materialize creates at most one immutable occurrence for an instant. The
// caller supplies the instant selected by SKIP/RUN_LATEST semantics; no wall
// clock is used in identity generation.
func (s Scheduler) Materialize(ctx context.Context, schedule Schedule, at time.Time) (Occurrence, error) {
	if err := s.Validate(); err != nil {
		return Occurrence{}, err
	}
	if err := schedule.Validate(); err != nil {
		return Occurrence{}, err
	}
	o := NewOccurrence(schedule, at)
	if err := s.Store.SaveOccurrence(ctx, o); err != nil {
		return Occurrence{}, err
	}
	return o, nil
}
func (s Scheduler) Claim(ctx context.Context) (Occurrence, bool, error) {
	if err := s.Validate(); err != nil {
		return Occurrence{}, false, err
	}
	return s.Store.ClaimDue(ctx, s.Now().UTC(), s.WorkerID, s.Lease)
}

// MissedInstants is deliberately bounded and explicit: SKIP yields no work;
// RUN_LATEST yields only the latest occurrence not later than now.
func MissedInstants(spec ScheduleSpec, from, now time.Time, max int) ([]time.Time, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if max < 1 {
		return nil, fmt.Errorf("missed-run bound is required")
	}
	if spec.Missed == MissedSkip {
		return nil, nil
	}
	result := make([]time.Time, 0, max)
	cursor := from.Add(-time.Nanosecond)
	for len(result) < max {
		next, ok := spec.Recurrence.Next(cursor)
		if !ok || next.After(now) {
			break
		}
		result = append(result, next)
		cursor = next
	}
	if len(result) == 0 {
		return nil, nil
	}
	return []time.Time{result[len(result)-1]}, nil
}
