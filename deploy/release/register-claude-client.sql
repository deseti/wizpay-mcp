-- Migration owner ONLY, after approval. No client secret for this public client.
-- psql -v client_id=OWNER_CHOSEN_PUBLIC_ID -v callback_uri=EXACT_VERIFIED_CALLBACK -f ...
\set ON_ERROR_STOP on
BEGIN;
SELECT set_config('wizpay.client_id', :'client_id', true);
SELECT set_config('wizpay.callback_uri', :'callback_uri', true);
DO $$ BEGIN
 IF current_setting('wizpay.client_id') !~ '^[A-Za-z0-9._~-]{1,128}$' THEN
  RAISE EXCEPTION 'Valid reviewed public client identifier required';
 END IF;
 IF current_setting('wizpay.callback_uri') <> 'https://claude.ai/api/mcp/auth_callback' THEN
  RAISE EXCEPTION 'Hosted Claude callback must match reviewed official documentation exactly';
 END IF;
END $$;
INSERT INTO oauth_clients(client_id,name,client_type,auth_method,status,redirect_uris,scopes,resources)
VALUES(:'client_id','Claude','public','none','ACTIVE',ARRAY[:'callback_uri'],ARRAY['mcp:read'],ARRAY['https://mcp.wizpay.xyz/mcp']);
-- No conflict overwrite; revocation is terminal and registrations require review.
COMMIT;
