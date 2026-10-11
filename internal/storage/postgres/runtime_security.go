package postgres

import (
	"context"
	"errors"
)

// CheckReadOnlyRuntimeRole refuses migration/admin credentials in the public process.
func (s *Store) CheckReadOnlyRuntimeRole(ctx context.Context) error {
	bounded, cancel, e := s.queryContext(ctx)
	if e != nil {
		return e
	}
	defer cancel()
	var safe bool
	e = s.pool.QueryRow(bounded, `SELECT NOT (r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls)
 AND NOT EXISTS(SELECT 1 FROM pg_auth_members WHERE member=r.oid)
 AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relowner=r.oid)
 AND NOT EXISTS(SELECT 1 FROM pg_proc WHERE proowner=r.oid)
 AND NOT EXISTS(SELECT 1 FROM pg_database WHERE datname=current_database() AND datdba=r.oid)
 AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m')
 AND c.relname NOT IN ('oauth_transactions','oauth_sessions','oauth_consents','oauth_codes','oauth_access_tokens','browser_sessions','siwe_challenges','siwe_authentications','wallet_enrollment_proofs','oauth_audit','mcp_access_audit')
 AND has_table_privilege(current_user,c.oid,'INSERT,UPDATE,DELETE,TRUNCATE'))
 AND NOT has_schema_privilege(current_user,'public','CREATE')
 AND NOT has_column_privilege(current_user,'tenants','status','UPDATE')
 AND NOT has_column_privilege(current_user,'identities','status','UPDATE')
 AND NOT has_column_privilege(current_user,'wallet_bindings','status','UPDATE')
 AND NOT has_column_privilege(current_user,'oauth_clients','status','UPDATE')
 AND NOT has_table_privilege(current_user,'wallet_enrollment_approvals','INSERT')
 AND NOT has_function_privilege(current_user,'admin_onboarding(text,text,text,text,text,text,text,text,text,timestamptz)','EXECUTE')
 AND NOT has_function_privilege(current_user,'admin_activate_wallet(text,text,text,text,text,text,timestamptz)','EXECUTE')
 FROM pg_roles r WHERE rolname=current_user`).Scan(&safe)
	if e != nil || !safe {
		return errors.New("public runtime requires isolated read-only database role")
	}
	return nil
}
