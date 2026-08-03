package postgres

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

var integrationPool *pgxpool.Pool
var integrationStore *Store
var integrationURL string

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.WithDatabase("wizpay_mcp_test"), tcpostgres.WithUsername("wizpay_test"), tcpostgres.WithPassword("local-test-password"), tcpostgres.BasicWaitStrategies())
	if err != nil {
		panic(err)
	}
	integrationURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}
	integrationPool, err = pgxpool.New(ctx, integrationURL)
	if err != nil {
		panic(err)
	}
	if err := Migrate(ctx, integrationPool); err != nil {
		panic(err)
	}
	integrationStore, err = NewStore(integrationPool, 10*time.Second, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err != nil {
		panic(err)
	}
	code := m.Run()
	integrationPool.Close()
	_ = container.Terminate(context.Background())
	os.Exit(code)
}

func TestFreshAndRepeatableMigrations(t *testing.T) {
	if err := Migrate(context.Background(), integrationPool); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	var count int
	if err := integrationPool.QueryRow(context.Background(), `SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("migration count = %d, want 2", count)
	}
}
