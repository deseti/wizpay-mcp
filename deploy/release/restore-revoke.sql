-- OWNER INCIDENT OPERATION ONLY after restore into the isolated recovery DB.
-- Invalidate restored bearer/browser/consent authority before any cutover.
BEGIN;
DO $$ BEGIN
 IF current_database() <> 'wizpay_mcp_restore' THEN
  RAISE EXCEPTION 'Isolated recovery database required';
 END IF;
END $$;
UPDATE browser_sessions SET revoked_at=COALESCE(revoked_at,clock_timestamp());
UPDATE oauth_sessions SET revoked_at=COALESCE(revoked_at,clock_timestamp());
UPDATE oauth_consents SET revoked_at=COALESCE(revoked_at,clock_timestamp());
UPDATE oauth_access_tokens SET revoked_at=COALESCE(revoked_at,clock_timestamp());
-- Codes and challenges cannot redeem after their sessions are revoked. Do not
-- fabricate challenge consumption timestamps or rewrite immutable evidence.
COMMIT;
