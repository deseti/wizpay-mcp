package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/deseti/wizpay-mcp/db/migrations"
)

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if ctx == nil || pool == nil {
		return fmt.Errorf("migration context and pool are required")
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version bigint PRIMARY KEY CHECK (version > 0), applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return mapDatabaseError(err)
	}
	entries, err := fs.Glob(dbmigrations.Files, "*.up.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(entries)
	for _, name := range entries {
		versionText, _, ok := strings.Cut(name, "_")
		if !ok {
			return fmt.Errorf("invalid migration name %q", name)
		}
		version, err := strconv.ParseInt(versionText, 10, 64)
		if err != nil || version <= 0 {
			return fmt.Errorf("invalid migration version %q", name)
		}
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists); err != nil {
			return mapDatabaseError(err)
		}
		if exists {
			continue
		}
		body, err := dbmigrations.Files.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %q: %w", name, err)
		}
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return mapDatabaseError(err)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %d: %w", version, mapDatabaseError(err))
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, version); err != nil {
			_ = tx.Rollback(ctx)
			return mapDatabaseError(err)
		}
		if err := tx.Commit(ctx); err != nil {
			return mapDatabaseError(err)
		}
	}
	return nil
}
