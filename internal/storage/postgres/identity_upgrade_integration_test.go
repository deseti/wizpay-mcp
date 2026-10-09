package postgres

import (
	"context"
	"fmt"
	dbmigrations "github.com/deseti/wizpay-mcp/db/migrations"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWP1MigrationPreservesPreWP1Intent(t *testing.T) {
	ctx := context.Background()
	f := createBaseFixture(t, false)
	name := unique("wp1_upgrade")
	parsed, err := url.Parse(integrationURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/postgres"
	admin, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err = admin.Exec(ctx, fmt.Sprintf(`CREATE DATABASE "%s"`, name)); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec(ctx, fmt.Sprintf(`DROP DATABASE "%s" WITH (FORCE)`, name)) }()
	parsed.Path = "/" + name
	pool, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	names, err := fs.Glob(dbmigrations.Files, "*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	for _, file := range names {
		version, _ := strconv.Atoi(strings.Split(file, "_")[0])
		if version >= 7 {
			continue
		}
		body, err := dbmigrations.Files.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(body)); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, version); err != nil {
			t.Fatal(err)
		}
	}
	store, err := NewStore(pool, 10*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	// Seed the historical schema explicitly. Current sqlc tenant queries return
	// migration-010 status, which does not exist in this pre-WP1 database.
	if _, err = pool.Exec(ctx, `INSERT INTO tenants(tenant_id,created_at) VALUES($1,$2)`, f.scope.TenantID(), fixtureNow); err != nil {
		t.Fatal(err)
	}
	identity, err := auth.NewIdentityWithSubject(f.scope.ActorID(), f.identity.Provider(), f.identity.ProviderSubject(), auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateIdentity(ctx, f.scope, identity); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateBinding(ctx, f.scope, f.binding); err != nil {
		t.Fatal(err)
	}
	a, err := intentCreateParams(f.scope, f.intent)
	if err != nil {
		t.Fatal(err)
	}
	args := []any{a.TenantID, a.IntentID, a.IntentVersion, a.ClientRequestID, a.Nonce, a.IntentType, a.UserID, a.IdentityProvider, a.ProviderUserReference, a.WalletBindingID, a.WalletBindingVersion, a.WalletID, a.WalletAddress, a.ChainID, a.Network, a.Financial, a.RouteType, a.RouteReference, a.RouteVersion, a.ConstraintDeadline, a.PolicyReference, a.CreatedAt, a.ExpiresAt, a.Status, a.IntentDigest, a.OperationKey, a.OperationVersion, a.LifecycleVersion}
	placeholders := make([]string, len(args))
	for j := range args {
		placeholders[j] = fmt.Sprintf("$%d", j+1)
	}
	if _, err = pool.Exec(ctx, "INSERT INTO intents VALUES ("+strings.Join(placeholders, ",")+")", args...); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateApproval(ctx, f.scope, f.approval); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	// Migration must preserve the legacy tenant and never approve SIWE access.
	var tenantStatus string
	var tenantCreated time.Time
	if err = pool.QueryRow(ctx, `SELECT status,created_at FROM tenants WHERE tenant_id=$1`, f.scope.TenantID()).Scan(&tenantStatus, &tenantCreated); err != nil {
		t.Fatal(err)
	}
	if tenantStatus != "INACTIVE" || !tenantCreated.Equal(fixtureNow) {
		t.Fatal("migration changed historical tenant or activated onboarding")
	}
	// The current repository now operates against its intended, migrated schema.
	// An idempotent tenant write must also leave onboarding authority inactive.
	if _, err = store.CreateTenant(ctx, storage.Tenant{TenantID: f.scope.TenantID(), CreatedAt: fixtureNow.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status,created_at FROM tenants WHERE tenant_id=$1`, f.scope.TenantID()).Scan(&tenantStatus, &tenantCreated); err != nil {
		t.Fatal(err)
	}
	if tenantStatus != "INACTIVE" || !tenantCreated.Equal(fixtureNow) {
		t.Fatal("tenant persistence changed lifecycle or historical creation time")
	}
	restored, err := store.FindIntentByID(ctx, f.scope, f.intent.IntentID())
	if err != nil {
		t.Fatal(err)
	}
	approval, err := store.FindApprovalByID(ctx, f.scope, f.approval.ApprovalID())
	if err != nil {
		t.Fatal(err)
	}
	if approval.IntentDigest() != f.approval.IntentDigest() || approval.WalletBindingVersion() != f.approval.WalletBindingVersion() || approval.Status() != f.approval.Status() {
		t.Fatal("migration changed historical approval")
	}
	if restored.Digest() != f.intent.Digest() || restored.Ownership() != f.intent.Ownership() {
		t.Fatal("migration changed frozen legacy identity")
	}
}
