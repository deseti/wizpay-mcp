package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReleaseRuntimeSchemaCheck(t *testing.T) {
	if err := CheckSchema(context.Background(), integrationPool); err != nil {
		t.Fatal(err)
	}
}

// Role grants are checked against the real migrated disposable database, never
// the owner's Compose stack. The role and all grants roll back with the test.
func TestReleaseRuntimeRoleLocksWithoutAuthorityMutation(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	role := fmt.Sprintf("wp3d_runtime_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{role}.Sanitize()
	if _, err = tx.Exec(ctx, "CREATE ROLE "+quoted+" NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile("../../../deploy/release/runtime-role.sql")
	if err != nil {
		t.Fatal(err)
	}
	grants := strings.ReplaceAll(string(sql), "wizpay_runtime", quoted)
	grants = strings.ReplaceAll(grants, "BEGIN;", "")
	grants = strings.ReplaceAll(grants, "COMMIT;", "")
	if _, err = tx.Exec(ctx, grants); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		table, column string
		allowed       bool
	}{
		{"identities", "updated_at", true}, {"identities", "status", false},
		{"identities", "user_id", false}, {"identities", "lifecycle_version", false},
		{"tenants", "created_at", true}, {"tenants", "status", false},
		{"wallet_bindings", "binding_id", true}, {"wallet_bindings", "version", false},
		{"wallet_bindings", "status", false}, {"oauth_clients", "client_id", true},
		{"oauth_clients", "status", false}, {"intents", "status", false},
	} {
		var allowed bool
		if err = tx.QueryRow(ctx, "SELECT has_column_privilege($1,$2,$3,'UPDATE')", role, check.table, check.column).Scan(&allowed); err != nil {
			t.Fatal(err)
		}
		if allowed != check.allowed {
			t.Errorf("%s.%s update privilege = %v", check.table, check.column, allowed)
		}
	}
	if _, err = tx.Exec(ctx, "SET LOCAL ROLE "+quoted); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"tenants", "identities", "wallet_bindings", "oauth_clients"} {
		rows, err := tx.Query(ctx, "SELECT 1 FROM "+pgx.Identifier{table}.Sanitize()+" WHERE false FOR UPDATE")
		if err != nil {
			t.Fatalf("authority lock permission on %s: %v", table, err)
		}
		rows.Close()
	}
}
