package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/deseti/wizpay-mcp/internal/onboarding"
	"github.com/jackc/pgx/v5"
)

// Uses only the package's disposable test database. TestMain starts Docker;
// compile-only checks must not be reported as executing these tests.
func onboardingLogin(t *testing.T, tenant string) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	role := unique("onboarding_admin")
	quoted := pgx.Identifier{role}.Sanitize()
	for _, sql := range []string{
		"CREATE ROLE " + quoted + " LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS",
		"GRANT USAGE ON SCHEMA public TO " + quoted,
		"GRANT EXECUTE ON FUNCTION admin_onboarding(text,text,text,text,text,text,text,text,text,timestamptz) TO " + quoted,
	} {
		if _, err := integrationPool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := integrationPool.Exec(ctx, `INSERT INTO onboarding_administrators VALUES($1,$2,clock_timestamp()+interval '1 hour')`, role, tenant); err != nil {
		t.Fatal(err)
	}
	c, err := pgx.Connect(ctx, integrationURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Exec(ctx, "SET SESSION AUTHORIZATION "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close(ctx) })
	return c
}
func applyOnboarding(c *pgx.Conn, r onboarding.Request) error {
	_, err := c.Exec(context.Background(), `SELECT admin_onboarding($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, r.Action, r.RequestID, r.TenantID, r.UserID, r.BindingID, r.Issuer, r.Subject, r.Address, r.ReviewReference, r.ExpiresAt)
	return err
}
func TestOnboardingLeastPrivilegePendingAndReplay(t *testing.T) {
	ctx := context.Background()
	tenant := unique("pending")
	c := onboardingLogin(t, tenant)
	r := onboarding.Request{Action: "STAGE", RequestID: unique("request"), TenantID: tenant, UserID: unique("user"), BindingID: unique("binding"), Issuer: "test-only-issuer", Subject: unique("subject"), Address: "0x0000000000000000000000000000000000000001", ReviewReference: "test-only-pending-review", ExpiresAt: time.Now().Add(time.Minute)}
	if err := applyOnboarding(c, r); err != nil {
		t.Fatal(err)
	}
	var pending bool
	if err := integrationPool.QueryRow(ctx, `SELECT t.status='INACTIVE' AND i.status='UNKNOWN' AND w.status='PENDING' AND w.verification_reference='' AND w.verified_at IS NULL FROM tenants t JOIN identities i USING(tenant_id) JOIN wallet_bindings w USING(tenant_id,user_id) WHERE t.tenant_id=$1`, tenant).Scan(&pending); err != nil || !pending {
		t.Fatal("enrollment granted authority", err)
	}
	for _, mutation := range []func(*onboarding.Request){
		func(r *onboarding.Request) {}, // Exact replay is rejected, not a second write.
		func(r *onboarding.Request) { r.Action = "ACTIVATE"; r.RequestID = unique("request") },
		func(r *onboarding.Request) { r.TenantID = unique("other") },
		func(r *onboarding.Request) {
			r.ExpiresAt = time.Now().Add(-time.Second)
			r.RequestID = unique("request")
		},
		func(r *onboarding.Request) { r.ExpiresAt = time.Now().Add(time.Hour); r.RequestID = unique("request") },
		func(r *onboarding.Request) {
			r.RequestID = unique("request")
			r.UserID = unique("other")
			r.BindingID = unique("other")
			r.Subject = unique("other")
		}, // Duplicate wallet.
	} {
		bad := r
		mutation(&bad)
		if applyOnboarding(c, bad) == nil {
			t.Fatal("unsafe enrollment accepted")
		}
	}
	for _, sql := range []string{
		`UPDATE tenants SET status='ACTIVE'`,
		`UPDATE identities SET status='ACTIVE'`,
		`UPDATE wallet_bindings SET status='ACTIVE'`,
		`INSERT INTO onboarding_administrators VALUES(session_user,'other',clock_timestamp()+interval '1 hour')`,
		`DELETE FROM onboarding_administration_audit`,
		`SELECT * FROM oauth_access_tokens`,
		`SELECT * FROM intents`,
	} {
		if _, err := c.Exec(ctx, sql); err == nil {
			t.Fatal("administrator escaped function boundary")
		}
	}
	var count int
	if err := integrationPool.QueryRow(ctx, `SELECT count(*) FROM onboarding_administration_audit WHERE tenant_id=$1`, tenant).Scan(&count); err != nil || count != 1 {
		t.Fatal("denied writes left audit/state changes", err)
	}
	if _, err := integrationPool.Exec(ctx, `UPDATE onboarding_administrators SET expires_at=clock_timestamp()-interval '1 second' WHERE tenant_id=$1`, tenant); err != nil {
		t.Fatal(err)
	}
	r = onboarding.Request{Action: "REVOKE_TENANT", RequestID: unique("request"), TenantID: tenant, ReviewReference: "review-revoke", ExpiresAt: time.Now().Add(time.Minute)}
	if applyOnboarding(c, r) == nil {
		t.Fatal("expired delegation accepted")
	}
}

func TestOnboardingRevocationInvalidatesSIWE(t *testing.T) {
	for _, action := range []string{"REVOKE_TENANT", "REVOKE_IDENTITY", "REVOKE_BINDING"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			f := newSIWEFixture(t, false)
			raw, err := f.service.VerifyWallet(ctx, f.raw, f.proof, f.c.ID, siweSignature(t, f.c.Message))
			if err != nil {
				t.Fatal(err)
			}
			v, err := f.store.FindBrowserSession(ctx, oauth.Digest(raw))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.FindBrowserAuthentication(ctx, v); err != nil {
				t.Fatal(err)
			}
			c := onboardingLogin(t, f.tenant)
			r := onboarding.Request{Action: action, RequestID: unique("revoke"), TenantID: f.tenant, ReviewReference: "test-only-revocation", ExpiresAt: time.Now().Add(time.Minute)}
			if action != "REVOKE_TENANT" {
				r.UserID = v.UserID
			}
			if action == "REVOKE_BINDING" {
				r.BindingID = f.binding
			}
			bad := r
			bad.TenantID = unique("other")
			if applyOnboarding(c, bad) == nil {
				t.Fatal("cross-tenant revocation accepted")
			}
			if action != "REVOKE_TENANT" {
				bad = r
				bad.UserID = unique("other")
				if applyOnboarding(c, bad) == nil {
					t.Fatal("cross-user revocation accepted")
				}
			}
			if err = applyOnboarding(c, r); err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.FindBrowserAuthentication(ctx, v); err == nil {
				t.Fatal("revoked SIWE authority accepted")
			}
			if applyOnboarding(c, r) == nil {
				t.Fatal("revocation replay accepted")
			}
			var sql string
			switch action {
			case "REVOKE_TENANT":
				sql = `UPDATE tenants SET status='ACTIVE' WHERE tenant_id=$1`
			case "REVOKE_IDENTITY":
				sql = `UPDATE identities SET status='ACTIVE',lifecycle_version=lifecycle_version+1 WHERE tenant_id=$1`
			case "REVOKE_BINDING":
				sql = `UPDATE wallet_bindings SET status='ACTIVE',revoked_at=NULL,version=version+1 WHERE tenant_id=$1 AND status='REVOKED'`
			}
			if _, err = integrationPool.Exec(ctx, sql, f.tenant); err == nil {
				t.Fatal("terminal authority reactivated")
			}
		})
	}
}

func TestOnboardingHasNoPublicExecute(t *testing.T) {
	var allowed bool
	err := integrationPool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_proc p, LATERAL aclexplode(p.proacl) a WHERE p.oid='admin_onboarding(text,text,text,text,text,text,text,text,text,timestamptz)'::regprocedure AND a.grantee=0 AND a.privilege_type='EXECUTE')`).Scan(&allowed)
	if err != nil || allowed {
		t.Fatal("public administration privilege", err)
	}
}
