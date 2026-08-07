package runtimeworker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/autonomy"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

type DurableStore interface {
	ClaimNextAutonomyDue(context.Context, string, time.Time, time.Duration) (storage.Scope, autonomy.Occurrence, bool, error)
	BlockAutonomyOccurrence(context.Context, storage.Scope, autonomy.Occurrence, autonomy.ReasonCode) error
}
type Processor interface {
	ProcessAutonomyOccurrence(context.Context, storage.Scope, autonomy.Occurrence) error
}
type WorkerConfig struct {
	WorkerID                     string
	LeaseDuration, RetryInterval time.Duration
	Enabled                      bool
}
type Worker struct {
	store     DurableStore
	processor Processor
	config    WorkerConfig
	now       func() time.Time
	sleep     func(context.Context, time.Duration) error
}
type OccurrenceError struct {
	Err                error
	Reason             autonomy.ReasonCode
	SubmissionPossible bool
}

func (e OccurrenceError) Error() string {
	if e.Err == nil {
		return "autonomous occurrence failed"
	}
	return e.Err.Error()
}
func (e OccurrenceError) Unwrap() error { return e.Err }

func NewWorker(store DurableStore, processor Processor, config WorkerConfig, now func() time.Time, sleep func(context.Context, time.Duration) error) (*Worker, error) {
	if store == nil || processor == nil || now == nil || sleep == nil || config.WorkerID == "" || config.LeaseDuration <= 0 || config.RetryInterval <= 0 {
		return nil, fmt.Errorf("autonomy worker dependencies are required")
	}
	return &Worker{store: store, processor: processor, config: config, now: now, sleep: sleep}, nil
}
func (w *Worker) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("worker context is required")
	}
	for {
		if !w.config.Enabled {
			if err := w.sleep(ctx, w.config.RetryInterval); err != nil {
				return err
			}
			continue
		}
		scope, occ, ok, err := w.store.ClaimNextAutonomyDue(ctx, w.config.WorkerID, w.now().UTC(), w.config.LeaseDuration)
		if err != nil {
			return err
		}
		if !ok {
			if err := w.sleep(ctx, w.config.RetryInterval); err != nil {
				return err
			}
			continue
		}
		if err := w.processor.ProcessAutonomyOccurrence(ctx, scope, occ); err != nil {
			var occurrenceErr OccurrenceError
			if errors.As(err, &occurrenceErr) && occurrenceErr.SubmissionPossible {
				return err
			}
			reason := autonomy.ReasonProviderUnavailable
			if errors.As(err, &occurrenceErr) && occurrenceErr.Reason.Valid() {
				reason = occurrenceErr.Reason
			}
			if blockErr := w.store.BlockAutonomyOccurrence(ctx, scope, occ, reason); blockErr != nil {
				return blockErr
			}
			continue
		}
	}
}

type UnavailableProcessor struct{ Store DurableStore }

func (p UnavailableProcessor) ProcessAutonomyOccurrence(ctx context.Context, scope storage.Scope, occ autonomy.Occurrence) error {
	return p.Store.BlockAutonomyOccurrence(ctx, scope, occ, autonomy.ReasonProviderUnavailable)
}
