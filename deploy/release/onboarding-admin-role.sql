-- Operator-only, migration owner, AFTER migration 011. Not run by this task.
\set ON_ERROR_STOP on
-- Required psql variables: admin_role (an existing dedicated LOGIN role),
-- tenant_id (one approved tenant), delegation_expires (reviewed UTC timestamp).
-- Create credentials through protected owner tooling, never this file or argv.
-- Use a fresh NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS role
-- with no memberships. Do not use the runtime or migration login.
BEGIN;
SELECT set_config('wizpay.admin_role', :'admin_role', true);
DO $$
DECLARE admin_oid oid;
BEGIN
 SELECT oid INTO admin_oid FROM pg_roles WHERE rolname=current_setting('wizpay.admin_role')
  AND rolcanlogin AND NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole
  AND NOT rolreplication AND NOT rolbypassrls;
 IF admin_oid IS NULL OR EXISTS(SELECT 1 FROM pg_auth_members WHERE member=admin_oid)
    OR current_setting('wizpay.admin_role') IN ('wizpay_runtime','wizpay_migrator',session_user) THEN
  RAISE EXCEPTION 'Dedicated least-privilege administrator required';
 END IF;
 -- A reused role may retain column privileges or ownership despite table REVOKE.
 IF EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m','S') AND
   (c.relowner=admin_oid OR has_any_column_privilege(admin_oid,c.oid,'SELECT,INSERT,UPDATE,REFERENCES')))
   OR EXISTS(SELECT 1 FROM pg_proc WHERE proowner=admin_oid)
   OR EXISTS(SELECT 1 FROM pg_namespace WHERE nspowner=admin_oid)
   OR EXISTS(SELECT 1 FROM pg_database WHERE datdba=admin_oid)
   OR has_database_privilege(admin_oid,current_database(),'CREATE') THEN
  RAISE EXCEPTION 'Fresh administrator without existing object authority required';
 END IF;
END $$;
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM :"admin_role";
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM :"admin_role";
REVOKE CREATE ON SCHEMA public FROM :"admin_role";
GRANT USAGE ON SCHEMA public TO :"admin_role";
GRANT EXECUTE ON FUNCTION admin_onboarding(text,text,text,text,text,text,text,text,text,timestamptz) TO :"admin_role";
GRANT EXECUTE ON FUNCTION admin_activate_wallet(text,text,text,text,text,text,timestamptz) TO :"admin_role";
-- A duplicate delegation fails; renewal/reassignment needs explicit owner review.
INSERT INTO onboarding_administrators(login_role,tenant_id,expires_at)
 VALUES(:'admin_role',:'tenant_id',:'delegation_expires'::timestamptz);
COMMIT;
