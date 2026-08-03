package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/deseti/wizpay-mcp/internal/app"
	"github.com/deseti/wizpay-mcp/internal/config"
	"github.com/deseti/wizpay-mcp/internal/logging"
	storagepostgres "github.com/deseti/wizpay-mcp/internal/storage/postgres"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("startup_failed", "error", err)
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

	databaseConfig, err := storagepostgres.LoadConfig(os.LookupEnv)
	if err != nil {
		return err
	}
	migrationContext, cancelMigration := context.WithTimeout(context.Background(), databaseConfig.ConnectTimeout)
	migrator, err := storagepostgres.Open(migrationContext, databaseConfig.MigrationConfig(), logger)
	if err != nil {
		cancelMigration()
		return err
	}
	if err := storagepostgres.Migrate(migrationContext, migrator.Pool()); err != nil {
		migrator.Close()
		cancelMigration()
		return err
	}
	migrator.Close()
	cancelMigration()

	databaseContext, cancelDatabase := context.WithTimeout(context.Background(), databaseConfig.ConnectTimeout)
	defer cancelDatabase()
	database, err := storagepostgres.Open(databaseContext, databaseConfig, logger)
	if err != nil {
		return err
	}
	defer database.Close()

	server, err := app.NewServerWithReadiness(cfg, logger, database)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return server.Run(ctx)
}
