package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBackupRestorePreservesPersistenceInvariants(t *testing.T) {
	createBaseFixture(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	dumpPath := filepath.Join(t.TempDir(), "wizpay-mcp-test.dump")
	if output, err := exec.CommandContext(ctx, "pg_dump", "--dbname", integrationURL, "--format=custom", "--no-owner", "--no-privileges", "--file", dumpPath).CombinedOutput(); err != nil {
		t.Fatalf("pg_dump: %v: %s", err, output)
	}
	restoreDB := unique("restore")
	parsed, err := url.Parse(integrationURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/postgres"
	adminPool, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, fmt.Sprintf(`CREATE DATABASE "%s"`, restoreDB)); err != nil {
		adminPool.Close()
		t.Fatal(err)
	}
	adminPool.Close()
	t.Cleanup(func() {
		admin, _ := pgxpool.New(context.Background(), parsed.String())
		if admin != nil {
			_, _ = admin.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, restoreDB))
			admin.Close()
		}
	})
	parsed.Path = "/" + restoreDB
	restoreURL := parsed.String()
	if output, err := exec.CommandContext(ctx, "pg_restore", "--dbname", restoreURL, "--no-owner", "--no-privileges", dumpPath).CombinedOutput(); err != nil {
		t.Fatalf("pg_restore: %v: %s", err, output)
	}
	restoredPool, err := pgxpool.New(ctx, restoreURL)
	if err != nil {
		t.Fatal(err)
	}
	defer restoredPool.Close()
	if err = Migrate(ctx, restoredPool); err != nil {
		t.Fatalf("migrate restored database: %v", err)
	}
	for _, table := range []string{"tenants", "identities", "wallet_bindings", "wallet_binding_versions", "intents", "approvals", "policies", "policy_evaluations", "policy_evaluation_findings", "execution_requests", "executions", "execution_revisions", "verification_evidence", "audit_records"} {
		var before, after int
		if err = integrationPool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&before); err != nil {
			t.Fatal(err)
		}
		if err = restoredPool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if before != after {
			t.Fatalf("%s count before/after = %d/%d", table, before, after)
		}
	}
	if _, err = restoredPool.Exec(ctx, `UPDATE audit_records SET new_state='tampered'`); err == nil {
		t.Fatal("restored audit append-only trigger missing")
	}
	if _, err = restoredPool.Exec(ctx, `INSERT INTO executions (tenant_id,execution_id,request_id,status,revision,created_at,updated_at) SELECT tenant_id,execution_id||'_duplicate',request_id,'CREATED',1,created_at,updated_at FROM executions LIMIT 1`); err == nil {
		t.Fatal("restored execution request uniqueness missing")
	}
	if _, err = restoredPool.Exec(ctx, `UPDATE execution_revisions SET status='FAILED'`); err == nil {
		t.Fatal("restored execution revision immutability missing")
	}
}
