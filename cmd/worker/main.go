package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/deseti/wizpay-mcp/internal/autonomy/runtimeworker"
	"github.com/deseti/wizpay-mcp/internal/config"
	"github.com/deseti/wizpay-mcp/internal/execution/runtime"
	"github.com/deseti/wizpay-mcp/internal/logging"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/providers/wiring"
	storagepostgres "github.com/deseti/wizpay-mcp/internal/storage/postgres"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("worker_startup_failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger, err := logging.New(os.Stdout, cfg.LogLevel)
	if err != nil {
		return err
	}
	logger.Info("configuration_loaded", "app_env", cfg.AppEnv, "log_level", cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The worker owns no schema. Migrations are applied by the server against a
	// separately managed migration login; the worker only opens the live pool.
	databaseConfig, err := storagepostgres.LoadConfig(os.LookupEnv)
	if err != nil {
		return err
	}
	databaseContext, cancelDatabase := context.WithTimeout(context.Background(), databaseConfig.ConnectTimeout)
	defer cancelDatabase()
	database, err := storagepostgres.Open(databaseContext, databaseConfig, logger)
	if err != nil {
		return err
	}
	defer database.Close()

	providerConfig, err := wiring.LoadConfig(os.LookupEnv)
	if err != nil {
		return err
	}

	// The reconciliation reference is read back from durable verification
	// evidence, so a restart or lease handoff never resubmits an execution that
	// already reached the provider.
	references, err := providers.NewEvidenceReferenceStore(database)
	if err != nil {
		return err
	}

	// Planner is deliberately nil: turning an approved intent into a concrete
	// transfer is domain capability logic that this phase does not implement.
	// Without it the provider adapter stays unconfigured and the worker idles.
	plane, err := wiring.Build(providerConfig, wiring.Dependencies{
		Planner:       nil,
		Authorization: providers.ContextAuthorizationSource{},
		References:    references,
		HTTPClient:    &http.Client{Timeout: 30 * time.Second},
		Now:           time.Now,
	})
	if err != nil {
		return err
	}

	runtimeConfig, err := loadRuntimeConfig(os.LookupEnv)
	if err != nil {
		return err
	}
	autonomyEnabled, err := boolValue(os.LookupEnv, "WIZPAY_AUTONOMY_ENABLED", false)
	if err != nil {
		return err
	}
	if autonomyEnabled {
		autonomyWorker, workerErr := runtimeworker.NewWorker(database, runtimeworker.UnavailableProcessor{Store: database}, runtimeworker.WorkerConfig{WorkerID: runtimeConfig.WorkerID, LeaseDuration: runtimeConfig.LeaseDuration, RetryInterval: runtimeConfig.RetryInterval, Enabled: true}, time.Now, sleep)
		if workerErr != nil {
			return workerErr
		}
		go func() {
			if runErr := autonomyWorker.Run(ctx); runErr != nil && !errors.Is(runErr, context.Canceled) {
				logger.Error("autonomy_worker_stopped", "error", runErr)
			}
		}()
		logger.Warn("autonomy_runtime_enabled_but_financial_assembly_unavailable", "reason", "typed planner and execution integration are not assembled; occurrences block safely")
	}
	worker, configured, err := wiring.BuildWorker(plane, database, runtimeConfig, time.Now, sleep)
	if err != nil {
		return err
	}
	if !configured {
		// Fail closed: with no configured provider adapter and chain verifier
		// there is nothing to drive, so the worker stays idle until shutdown
		// rather than substituting a permissive execution path.
		logger.Warn("execution_runtime_disabled", "reason", "provider plane is not fully configured")
		logger.Info("worker_started", "jobs_registered", 0)
		<-ctx.Done()
		logger.Info("worker_shutdown", "reason", "context_cancelled")
		return nil
	}

	logger.Info("worker_started", "worker_id", runtimeConfig.WorkerID, "lease_duration", runtimeConfig.LeaseDuration.String(), "retry_interval", runtimeConfig.RetryInterval.String())
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	logger.Info("worker_shutdown", "reason", "context_cancelled")
	return nil
}

func boolValue(lookup func(string) (string, bool), key string, fallback bool) (bool, error) {
	value, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true, nil
	case "0", "false", "no":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean", key)
	}
}

// loadRuntimeConfig reads the execution worker's lease and retry parameters. The
// worker ID must be stable per process so that lease ownership and fencing are
// attributable; it falls back to the host name and PID when not set.
func loadRuntimeConfig(lookup func(string) (string, bool)) (runtime.Config, error) {
	config := runtime.Config{
		WorkerID:      stringValue(lookup, "WIZPAY_WORKER_ID", defaultWorkerID()),
		LeaseDuration: durationValue(lookup, "WIZPAY_WORKER_LEASE_DURATION", 30*time.Second),
		RetryInterval: durationValue(lookup, "WIZPAY_WORKER_RETRY_INTERVAL", 5*time.Second),
	}
	if err := config.Validate(); err != nil {
		return runtime.Config{}, err
	}
	return config, nil
}

func defaultWorkerID() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "worker"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

// sleep waits for the interval or an earlier context cancellation.
func sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func stringValue(lookup func(string) (string, bool), key, fallback string) string {
	if value, found := lookup(key); found {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return fallback
}

func durationValue(lookup func(string) (string, bool), key string, fallback time.Duration) time.Duration {
	if value, found := lookup(key); found {
		if parsed, err := time.ParseDuration(strings.TrimSpace(value)); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}
