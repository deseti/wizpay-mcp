package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/storage"
)

// Worker polls PostgreSQL-backed work. Claim ownership and fencing remain in
// the repository; this loop has no correctness state of its own.
type Worker struct {
	service *Service
	store   storage.ExecutionRuntimeRepository
	config  Config
	now     func() time.Time
	sleep   func(context.Context, time.Duration) error
}

func NewWorker(service *Service, store storage.ExecutionRuntimeRepository, config Config, now func() time.Time, sleep func(context.Context, time.Duration) error) (*Worker, error) {
	if service == nil || store == nil || now == nil || sleep == nil {
		return nil, fmt.Errorf("worker dependencies are required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Worker{service: service, store: store, config: config, now: now, sleep: sleep}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("worker context is required")
	}
	for {
		claim, acquired, err := w.store.ClaimNextExecutionWork(ctx, w.config.WorkerID, w.now().UTC(), w.config.LeaseDuration)
		if err != nil {
			return err
		}
		if acquired {
			_, processErr := w.service.ProcessClaim(ctx, claim)
			_, releaseErr := w.store.ReleaseExecutionWork(ctx, claim, w.now().UTC().Add(w.config.RetryInterval))
			if processErr != nil {
				return processErr
			}
			if releaseErr != nil {
				return releaseErr
			}
			continue
		}
		if err := w.sleep(ctx, w.config.RetryInterval); err != nil {
			return err
		}
	}
}
