package wiring

import (
	"context"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/execution/runtime"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

// RuntimeStore is the persistence the execution worker requires. A single
// *postgres.Store satisfies all three repository ports; they are named
// separately here only to state exactly what the worker touches.
type RuntimeStore interface {
	storage.ExecutionRepository
	storage.VerificationEvidenceRepository
	storage.ExecutionRuntimeRepository
}

// BuildWorker assembles the Phase 9 execution worker from the provider plane and
// persistence.
//
// It fails closed. The worker is returned with configured=true only when the
// plane carries both a provider adapter and a chain-backed verifier; otherwise
// it returns configured=false and no worker, so the process idles rather than
// driving executions against a degraded plane. Because Phase 11 supplies no
// domain planner, the plane's adapter is always nil and this always reports
// unconfigured: no execution, and therefore no financial transaction, can be
// driven from this phase.
//
// A nil *circle.Adapter is deliberately checked through the plane fields rather
// than after assignment to the execution.Adapter interface, so a typed-nil can
// never be mistaken for a present adapter.
func BuildWorker(plane Plane, store RuntimeStore, config runtime.Config, now func() time.Time, sleep func(context.Context, time.Duration) error) (*runtime.Worker, bool, error) {
	if store == nil {
		return nil, false, fmt.Errorf("execution worker requires persistence")
	}
	if now == nil || sleep == nil {
		return nil, false, fmt.Errorf("execution worker requires a clock and a sleeper")
	}
	if plane.Adapter == nil || plane.Verifier == nil {
		// No configured provider adapter, or no chain-backed verifier, means no
		// execution may be driven. This is the Phase 11 state and it is safe: a
		// missing adapter is never replaced with a permissive stub.
		return nil, false, nil
	}
	service, err := runtime.NewService(store, store, store, plane.Adapter, plane.Verifier, config, now)
	if err != nil {
		return nil, false, err
	}
	worker, err := runtime.NewWorker(service, store, config, now, sleep)
	if err != nil {
		return nil, false, err
	}
	return worker, true, nil
}
