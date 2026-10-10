package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	dbmigrations "github.com/deseti/wizpay-mcp/db/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckSchema performs no DDL. Exact migration-set equality rejects a missing or
// newer schema; rollback must use a schema-compatible reviewed binary.
func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	names, err := fs.Glob(dbmigrations.Files, "*.up.sql")
	if err != nil {
		return fmt.Errorf("schema check failed")
	}
	expected := map[int64]bool{}
	for _, name := range names {
		text, _, _ := strings.Cut(name, "_")
		version, e := strconv.ParseInt(text, 10, 64)
		if e != nil {
			return fmt.Errorf("schema check failed")
		}
		expected[version] = true
	}
	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("schema check failed")
	}
	defer rows.Close()
	for rows.Next() {
		var version int64
		if rows.Scan(&version) != nil || !expected[version] {
			return fmt.Errorf("schema is incompatible")
		}
		delete(expected, version)
	}
	if rows.Err() != nil || len(expected) != 0 {
		return fmt.Errorf("schema is incompatible")
	}
	return nil
}
