package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/deseti/wizpay-mcp/internal/app"
	"github.com/deseti/wizpay-mcp/internal/auth"
	authjwt "github.com/deseti/wizpay-mcp/internal/auth/jwt"
	"github.com/deseti/wizpay-mcp/internal/config"
	"github.com/deseti/wizpay-mcp/internal/logging"
	"github.com/deseti/wizpay-mcp/internal/mcp/tools"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
	"github.com/deseti/wizpay-mcp/internal/services"
	"github.com/deseti/wizpay-mcp/internal/storage"
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

	var server *app.Server
	if cfg.Auth.Required {
		publicKeyPEM, readErr := os.ReadFile(cfg.Auth.PublicKeyFile)
		if readErr != nil {
			return readErr
		}
		publicKey, parseErr := authjwt.ParseRSAPublicKey(publicKeyPEM)
		if parseErr != nil {
			return parseErr
		}
		verifier, verifierErr := authjwt.NewVerifier(authjwt.Config{Issuer: cfg.Auth.Issuer, Audience: cfg.Auth.Audience, PublicKey: publicKey, AllowedAlgorithms: []string{"RS256"}, ClockSkew: cfg.Auth.ClockSkew}, time.Now)
		if verifierErr != nil {
			return verifierErr
		}
		middleware, middlewareErr := requestauth.NewMiddleware(verifier, requestauth.RepositoryResolver{Repository: database})
		if middlewareErr != nil {
			return middlewareErr
		}
		authorizer := auth.NewPermissionAuthorizer()
		foundationRegistry, registryErr := tools.NewFoundationRegistry(newFoundationBundle(database, authorizer, time.Now))
		if registryErr != nil {
			return registryErr
		}
		autonomyService := &services.PersistedAutonomyService{Repository: database, Authorizer: authorizer, Audit: database, Wallets: database, Now: time.Now, Enabled: cfg.AutonomousEnabled}
		autonomyRegistry, registryErr := tools.NewAutonomyRegistry(autonomyService)
		if registryErr != nil {
			return registryErr
		}
		registrations := append(foundationRegistry.Tools(), autonomyRegistry.Tools()...)
		server, err = app.NewAuthenticatedServer(cfg, logger, database, middleware.Wrap, registrations...)
	} else {
		server, err = app.NewServerWithReadiness(cfg, logger, database)
	}
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return server.Run(ctx)
}

type foundationRepository interface {
	storage.IntentRepository
	storage.ApprovalRepository
	storage.PolicyRepository
	storage.PolicyEvaluationRepository
	storage.ExecutionRepository
	storage.WalletBindingRepository
	storage.AuditRepository
}

func newFoundationBundle(repository foundationRepository, authorizer auth.Authorizer, now func() time.Time) services.Bundle {
	return services.Bundle{
		Intents:    &services.PersistedIntentService{Intents: repository, Wallets: repository, Authorizer: authorizer, Audit: repository, Now: now},
		Approvals:  &services.PersistedApprovalService{Approvals: repository, Intents: repository, Authorizer: authorizer, Audit: repository, Now: now},
		Policies:   &services.PersistedPolicyService{Intents: repository, Policies: repository, Evaluations: repository, Wallets: repository, Authorizer: authorizer, Now: now},
		Executions: &services.PersistedExecutionService{Intents: repository, Approvals: repository, Policies: repository, Evaluations: repository, Executions: repository, Wallets: repository, Authorizer: authorizer, Now: now},
	}
}
