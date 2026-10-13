package main

import (
	"fmt"
	"regexp"

	"github.com/deseti/wizpay-mcp/internal/siwe"
)

// References and SQL contain no keys or credentials. Bootstrap grants an
// expiring, tenant-scoped owner delegation and invokes the real STAGE boundary.
// Only server-verified ENROLL evidence can later authorize ACTIVATE.
type publicFixture struct {
	ID           string `json:"fixture_id"`
	Tenant       string `json:"tenant_id"`
	User         string `json:"user_id"`
	OtherUser    string `json:"other_user_id"`
	Binding      string `json:"binding_id"`
	OtherBinding string `json:"other_binding_id"`
	Client       string `json:"client_id"`
	Owner        string `json:"owner_role"`
	Address      string `json:"address"`
	OtherAddress string `json:"other_address"`
	SQL          string `json:"sql"`
}

var fixtureID = regexp.MustCompile(`^[0-9a-f]{64}$`)

func newFixture(id, address, otherAddress string) (publicFixture, error) {
	if !fixtureID.MatchString(id) {
		return publicFixture{}, siwe.ErrDenied
	}
	a, err := siwe.Address(address)
	if err != nil {
		return publicFixture{}, err
	}
	b, err := siwe.Address(otherAddress)
	if err != nil || a == b {
		return publicFixture{}, siwe.ErrDenied
	}
	f := publicFixture{ID: id, Tenant: "wp3d-browser-" + id, User: "browser-user-" + id,
		OtherUser: "other-browser-user-" + id, Binding: "binding-" + id, OtherBinding: "other-binding-" + id,
		Client: "browser-client-" + id, Owner: "wp3d_owner_" + id[:24], Address: a, OtherAddress: b}
	f.SQL = fmt.Sprintf(`\set ON_ERROR_STOP on
BEGIN;
DO $$ BEGIN IF current_database() <> 'wizpay_mcp_browser_e2e' THEN RAISE EXCEPTION 'wrong test database'; END IF; END $$;
CREATE ROLE %s NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
GRANT USAGE ON SCHEMA public TO %s;
GRANT EXECUTE ON FUNCTION admin_onboarding(text,text,text,text,text,text,text,text,text,timestamptz) TO %s;
GRANT EXECUTE ON FUNCTION admin_activate_wallet(text,text,text,text,text,text,timestamptz) TO %s;
INSERT INTO onboarding_administrators VALUES('%s','%s',clock_timestamp()+interval '3 hours');
INSERT INTO oauth_clients(client_id,name,client_type,auth_method,status,redirect_uris,scopes,resources) VALUES('%s','Synthetic Browser Client','public','none','ACTIVE',ARRAY['https://connect.wizpay.xyz/browser-e2e/callback'],ARRAY['mcp:read'],ARRAY['https://mcp.wizpay.xyz/mcp']);
SET SESSION AUTHORIZATION %s;
SELECT admin_onboarding('STAGE','stage-primary-%s','%s','%s','%s','browser-e2e','%s','%s','synthetic-owner-stage',clock_timestamp()+interval '5 minutes');
SELECT admin_onboarding('STAGE','stage-secondary-%s','%s','%s','%s','browser-e2e','%s','%s','synthetic-owner-stage',clock_timestamp()+interval '5 minutes');
RESET SESSION AUTHORIZATION;
DO $$ BEGIN
 IF (SELECT count(*) FROM wallet_bindings WHERE tenant_id='%s' AND status='PENDING' AND version=1 AND verification_reference='' AND verified_at IS NULL) <> 2
 OR NOT EXISTS(SELECT 1 FROM tenants WHERE tenant_id='%s' AND status='INACTIVE')
 OR (SELECT count(*) FROM identities WHERE tenant_id='%s' AND status='UNKNOWN') <> 2 THEN
 RAISE EXCEPTION 'synthetic staging did not remain pending'; END IF;
END $$;
COMMIT;
`, f.Owner, f.Owner, f.Owner, f.Owner, f.Owner, f.Tenant, f.Client, f.Owner,
		id, f.Tenant, f.User, f.Binding, f.User, f.Address,
		id, f.Tenant, f.OtherUser, f.OtherBinding, f.OtherUser, f.OtherAddress,
		f.Tenant, f.Tenant, f.Tenant)
	return f, nil
}
