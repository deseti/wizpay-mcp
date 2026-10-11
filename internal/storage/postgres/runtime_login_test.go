package postgres

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// Actual distinct nonowner login, freshly created only inside disposable TestMain DB.
func newRuntimeStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	role := unique("runtime_login")
	quoted := pgx.Identifier{role}.Sanitize()
	if _, err := integrationPool.Exec(ctx, "CREATE ROLE "+quoted+" LOGIN PASSWORD 'disposable-integration-only' NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile("../../../deploy/release/runtime-role.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationPool.Exec(ctx, strings.ReplaceAll(string(sql), "wizpay_runtime", quoted)); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(integrationURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	cfg.MinConns = 0
	cfg.ConnConfig.User = role
	cfg.ConnConfig.Password = "disposable-integration-only"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var login string
	var owner bool
	if err = pool.QueryRow(ctx, `SELECT session_user,(SELECT rolsuper FROM pg_roles WHERE rolname=session_user)`).Scan(&login, &owner); err != nil || owner || login != role {
		t.Fatal("runtime login separation", err)
	}
	for _, query := range []string{`UPDATE tenants SET status='ACTIVE'`, `UPDATE wallet_bindings SET status='ACTIVE'`, `INSERT INTO wallet_enrollment_approvals SELECT * FROM wallet_enrollment_approvals`, `SELECT admin_activate_wallet('x','x','x','x','x','x',now())`, `DELETE FROM mcp_access_audit`} {
		if _, err = pool.Exec(ctx, query); err == nil {
			t.Fatal("runtime escaped authority boundary")
		}
	}
	store, err := NewStore(pool, 10*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.CheckReadOnlyRuntimeRole(ctx); err != nil {
		t.Fatal(err)
	}
	if integrationStore.CheckReadOnlyRuntimeRole(ctx) == nil {
		t.Fatal("migration owner accepted as runtime")
	}
	store.UseDatabaseAuthenticationClock()
	return store
}

func TestAuthenticationClockFailsClosedWithoutFallback(t *testing.T) {
	store := newRuntimeStore(t)
	now := store.AuthenticationTime()
	if now.IsZero() {
		t.Fatal("database clock unavailable")
	}
	store.pool.Close()
	if !store.AuthenticationTime().IsZero() {
		t.Fatal("clock failure fell back to host time")
	}
}
