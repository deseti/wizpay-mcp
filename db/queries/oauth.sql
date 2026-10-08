-- name: FindOAuthClient :one
SELECT * FROM oauth_clients WHERE client_id=$1;

-- name: CreateOAuthClient :exec
INSERT INTO oauth_clients(client_id,name,client_type,auth_method,status,redirect_uris,scopes,resources) VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: CreateOAuthTransaction :exec
INSERT INTO oauth_transactions(transaction_id,client_id,redirect_uri,resource,scope,challenge,state,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: FindOAuthTransaction :one
SELECT * FROM oauth_transactions WHERE transaction_id=$1 AND completed_at IS NULL;

-- name: ValidateOAuthAccessToken :one
SELECT a.* FROM oauth_access_tokens a
JOIN oauth_clients c ON c.client_id=a.client_id
JOIN oauth_consents u ON u.tenant_id=a.tenant_id AND u.user_id=a.user_id AND u.consent_id=a.consent_id AND u.client_id=a.client_id AND u.resource=a.resource AND u.scope=a.scope AND u.session_id=a.session_id
JOIN oauth_sessions s ON s.tenant_id=a.tenant_id AND s.user_id=a.user_id AND s.session_id=a.session_id
JOIN identities i ON i.tenant_id=a.tenant_id AND i.user_id=a.user_id AND i.provider=a.identity_issuer AND i.provider_subject=a.subject
WHERE a.token_digest=$1 AND a.issuer='https://connect.wizpay.xyz' AND a.resource='https://mcp.wizpay.xyz/mcp'
AND a.revoked_at IS NULL AND a.issued_at<=$2 AND a.expires_at>$2
AND c.status='ACTIVE' AND c.client_type='public' AND c.auth_method='none'
AND a.scope=ANY(c.scopes) AND a.resource=ANY(c.resources)
AND u.revoked_at IS NULL AND u.created_at<=$2 AND u.expires_at>$2
AND NOT EXISTS(SELECT 1 FROM oauth_consent_wallets w WHERE w.tenant_id=u.tenant_id AND w.user_id=u.user_id AND w.consent_id=u.consent_id)
AND s.revoked_at IS NULL AND s.expires_at>$2 AND i.status='ACTIVE';
