-- WP3A: no identity provisioning, wallet evidence or authentication activation.
CREATE TABLE browser_sessions (
 session_digest text PRIMARY KEY CHECK(session_digest ~ '^[A-Za-z0-9_-]{43}$'),
 csrf_digest text NOT NULL CHECK(csrf_digest ~ '^[A-Za-z0-9_-]{43}$'),
 session_reference text NOT NULL UNIQUE,
 transaction_id text NOT NULL REFERENCES oauth_transactions(transaction_id),
 state text NOT NULL CHECK(state IN ('PENDING','AUTHENTICATED')),
 decision text NOT NULL DEFAULT 'OPEN' CHECK(decision IN ('OPEN','GRANTED','DENIED')),
 tenant_id text, user_id text, identity_issuer text, subject text, evidence_reference text,
 created_at timestamptz NOT NULL, expires_at timestamptz NOT NULL, revoked_at timestamptz,
 FOREIGN KEY(tenant_id,user_id) REFERENCES identities(tenant_id,user_id),
 CHECK(expires_at>created_at),
 CHECK((state='PENDING' AND tenant_id IS NULL AND user_id IS NULL AND identity_issuer IS NULL AND subject IS NULL AND evidence_reference IS NULL AND decision<>'GRANTED' AND expires_at<=created_at+interval '5 minutes') OR
       (state='AUTHENTICATED' AND tenant_id IS NOT NULL AND user_id IS NOT NULL AND identity_issuer IS NOT NULL AND subject IS NOT NULL AND evidence_reference IS NOT NULL AND length(evidence_reference) BETWEEN 1 AND 256 AND expires_at<=created_at+interval '30 minutes'))
);
CREATE INDEX browser_sessions_expiry_idx ON browser_sessions(expires_at);
CREATE FUNCTION reject_browser_session_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (to_jsonb(NEW)-'decision'-'revoked_at') IS DISTINCT FROM (to_jsonb(OLD)-'decision'-'revoked_at') OR
    (OLD.decision<>'OPEN' AND NEW.decision<>OLD.decision) OR
    (OLD.revoked_at IS NOT NULL AND NEW.revoked_at IS DISTINCT FROM OLD.revoked_at) THEN
  RAISE EXCEPTION 'Browser authority snapshot is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER browser_sessions_immutable BEFORE UPDATE ON browser_sessions FOR EACH ROW EXECUTE FUNCTION reject_browser_session_mutation();
-- Forward-only. Session records are rotated, never promoted in place.
