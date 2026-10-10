// Operator-run conservative cleanup: retained authentication evidence is untouched.
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
		os.Stderr.WriteString("session cleanup failed\n")
		os.Exit(1)
	}
}
func run() error {
	l, e := config.SecretLookup(os.LookupEnv, os.ReadFile)
	if e != nil {
		return e
	}
	c, e := postgres.LoadConfig(postgres.LookupEnv(l))
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, e := postgres.Open(ctx, c, slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if e != nil {
		return e
	}
	defer s.Close()
	return s.CleanupBrowserSessions(ctx)
}
