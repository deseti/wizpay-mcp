// Separate operator-run migration process; never mounted in the application.
package main

import (
	"context"
	"github.com/deseti/wizpay-mcp/internal/config"
	"github.com/deseti/wizpay-mcp/internal/storage/postgres"
	"log/slog"
	"os"
	"time"
)

func main() {
	if run() != nil {
		os.Stderr.WriteString("migration failed; inspect protected database diagnostics\n")
		os.Exit(1)
	}
}
func run() error {
	l, err := config.SecretLookup(os.LookupEnv, os.ReadFile)
	if err != nil {
		return err
	}
	c, err := postgres.LoadConfig(postgres.LookupEnv(l))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s, err := postgres.Open(ctx, c.MigrationConfig(), slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if err != nil {
		return err
	}
	defer s.Close()
	return postgres.Migrate(ctx, s.Pool())
}
