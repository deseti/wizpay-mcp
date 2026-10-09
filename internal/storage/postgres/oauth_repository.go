package postgres

import (
	"context"
	"slices"
	"time"

	"github.com/deseti/wizpay-mcp/internal/browser"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/deseti/wizpay-mcp/internal/storage/postgres/dbsqlc"
	"github.com/jackc/pgx/v5"
)

func (s *Store) RegisterOAuthClient(ctx context.Context, c oauth.Client) error {
	if c.Validate() != nil {
		return oauth.ErrDenied
	}
	bounded, cancel, e := s.queryContext(ctx)
	if e != nil {
		return e
	}
	defer cancel()
	return s.queries.CreateOAuthClient(bounded, dbsqlc.CreateOAuthClientParams{ClientID: c.ID, Name: c.Name, ClientType: c.Type, AuthMethod: c.AuthMethod, Status: c.Status, RedirectUris: c.RedirectURIs, Scopes: c.Scopes, Resources: c.Resources})
}
func (s *Store) FindOAuthClient(ctx context.Context, id string) (oauth.Client, error) {
	bounded, cancel, e := s.queryContext(ctx)
	if e != nil {
		return oauth.Client{}, e
	}
	defer cancel()
	c, e := s.queries.FindOAuthClient(bounded, id)
	return oauth.Client{ID: c.ClientID, Name: c.Name, Type: c.ClientType, AuthMethod: c.AuthMethod, Status: c.Status, RedirectURIs: c.RedirectUris, Scopes: c.Scopes, Resources: c.Resources}, e
}
func (s *Store) CreateOAuthTransaction(ctx context.Context, t oauth.Transaction) error {
	bounded, cancel, e := s.queryContext(ctx)
	if e != nil {
		return e
	}
	defer cancel()
	return s.queries.CreateOAuthTransaction(bounded, dbsqlc.CreateOAuthTransactionParams{TransactionID: t.ID, ClientID: t.Request.ClientID, RedirectUri: t.Request.RedirectURI, Resource: t.Request.Resource, Scope: t.Request.Scope, Challenge: t.Request.Challenge, State: t.Request.State, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt})
}
func (s *Store) FindOAuthTransaction(ctx context.Context, id string) (oauth.Transaction, error) {
	bounded, cancel, e := s.queryContext(ctx)
	if e != nil {
		return oauth.Transaction{}, e
	}
	defer cancel()
	t, e := s.queries.FindOAuthTransaction(bounded, id)
	return oauth.Transaction{ID: t.TransactionID, Request: oauth.AuthorizationRequest{ClientID: t.ClientID, RedirectURI: t.RedirectUri, Resource: t.Resource, Scope: t.Scope, ResponseType: "code", Challenge: t.Challenge, ChallengeMethod: "S256", State: t.State}, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt}, e
}

// oauthTx shares the established bounded transaction convention. Explicit row
// locks serialize redemption with revocation; rollback restores code eligibility.
func (s *Store) oauthTx(ctx context.Context, f func(context.Context, pgx.Tx) error) error {
	bounded, cancel, e := s.queryContext(ctx)
	if e != nil {
		return e
	}
	defer cancel()
	tx, e := s.pool.Begin(bounded)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	if e = f(bounded, tx); e != nil {
		return e
	}
	return tx.Commit(bounded)
}
func (s *Store) CompleteOAuthAuthorization(ctx context.Context, t oauth.Transaction, d oauth.BrowserDecision, k oauth.Code) error {
	return s.oauthTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var bs browser.Session
		if d.BrowserSessionReference != "" {
			var err error
			bs, err = s.lockBrowser(ctx, tx, "session_reference", d.BrowserSessionReference)
			if err != nil || bs.State != "AUTHENTICATED" || bs.Decision != "OPEN" || bs.TransactionID != t.ID || bs.Reference != k.SessionID || bs.TenantID != k.TenantID || bs.UserID != k.UserID || bs.Issuer != d.Principal.IdentityProvider() || bs.Subject != d.Principal.ProviderSubject() {
				return oauth.ErrDenied
			}
		}
		// Recheck the persisted transaction and registered client inside the lock.
		var client, redirect, resource, scope, challenge, state string
		var expires time.Time
		e := tx.QueryRow(ctx, `SELECT t.client_id,t.redirect_uri,t.resource,t.scope,t.challenge,t.state,t.expires_at FROM oauth_transactions t JOIN oauth_clients c ON c.client_id=t.client_id WHERE t.transaction_id=$1 AND t.completed_at IS NULL AND t.expires_at>$2 AND c.status='ACTIVE' AND c.client_type='public' AND c.auth_method='none' AND t.redirect_uri=ANY(c.redirect_uris) AND t.scope=ANY(c.scopes) AND t.resource=ANY(c.resources) FOR UPDATE OF t,c`, t.ID, k.IssuedAt).Scan(&client, &redirect, &resource, &scope, &challenge, &state, &expires)
		if e != nil || client != t.Request.ClientID || redirect != t.Request.RedirectURI || resource != t.Request.Resource || scope != t.Request.Scope || challenge != t.Request.Challenge || state != t.Request.State || k.TransactionID != t.ID || k.TenantID != d.Principal.TenantID() || k.UserID != d.Principal.ActorID() {
			return oauth.ErrDenied
		}
		var user string
		e = tx.QueryRow(ctx, `SELECT user_id FROM identities WHERE tenant_id=$1 AND user_id=$2 AND provider=$3 AND provider_subject=$4 AND status='ACTIVE' FOR UPDATE`, k.TenantID, k.UserID, d.Principal.IdentityProvider(), d.Principal.ProviderSubject()).Scan(&user)
		if e != nil {
			return oauth.ErrDenied
		}
		if e = checkSIWESession(ctx, tx, k.SessionID, k.TenantID, k.UserID); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO oauth_sessions(tenant_id,user_id,session_id,expires_at) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, k.TenantID, k.UserID, k.SessionID, d.SessionExpiresAt)
		if e != nil {
			return e
		}
		var sessionExpires time.Time
		e = tx.QueryRow(ctx, `SELECT expires_at FROM oauth_sessions WHERE tenant_id=$1 AND user_id=$2 AND session_id=$3 AND revoked_at IS NULL AND expires_at>$4 FOR UPDATE`, k.TenantID, k.UserID, k.SessionID, k.IssuedAt).Scan(&sessionExpires)
		if e != nil {
			return oauth.ErrDenied
		}
		if d.BrowserSessionReference != "" {
			now := s.now().UTC()
			if !now.Before(bs.ExpiresAt) || !now.Before(expires) || !now.Before(sessionExpires) || !now.Before(k.ExpiresAt) || !now.Before(d.Principal.ExpiresAt()) {
				return oauth.ErrDenied
			}
		}
		consentExpires := d.SessionExpiresAt
		if sessionExpires.Before(consentExpires) {
			consentExpires = sessionExpires
		}
		if d.Principal.ExpiresAt().Before(consentExpires) {
			consentExpires = d.Principal.ExpiresAt()
		}
		_, e = tx.Exec(ctx, `INSERT INTO oauth_consents(tenant_id,user_id,consent_id,client_id,resource,scope,session_id,created_at,expires_at,identity_issuer,subject) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, k.TenantID, k.UserID, k.ConsentID, client, resource, scope, k.SessionID, k.IssuedAt, consentExpires, d.Principal.IdentityProvider(), d.Principal.ProviderSubject())
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO oauth_codes(code_digest,transaction_id,tenant_id,user_id,consent_id,session_id,client_id,redirect_uri,resource,scope,challenge,issued_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, k.Digest, t.ID, k.TenantID, k.UserID, k.ConsentID, k.SessionID, client, redirect, resource, scope, challenge, k.IssuedAt, k.ExpiresAt)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE oauth_transactions SET completed_at=$2 WHERE transaction_id=$1`, t.ID, k.IssuedAt)
		if e != nil {
			return e
		}
		if d.BrowserSessionReference != "" {
			if _, e = tx.Exec(ctx, `UPDATE browser_sessions SET decision='GRANTED' WHERE session_reference=$1`, d.BrowserSessionReference); e != nil {
				return e
			}
		}
		return oauthAudit(ctx, tx, "AUTHORIZED", k.TenantID, k.UserID, client, k.ConsentID, k.IssuedAt)
	})
}
func oauthAudit(ctx context.Context, tx pgx.Tx, event, tenant, user, client, consent string, now time.Time) error {
	_, e := tx.Exec(ctx, `INSERT INTO oauth_audit(event_type,tenant_id,user_id,client_id,consent_id,occurred_at) VALUES($1,$2,$3,$4,$5,$6)`, event, tenant, user, client, consent, now)
	return e
}
func (s *Store) RedeemOAuthCode(ctx context.Context, r oauth.Redemption) (oauth.Token, error) {
	var out oauth.Token
	e := s.oauthTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var challenge, redirect string
		var codeIssued, codeExpires, consentCreated, consentExpires, sessionExpires time.Time
		e := tx.QueryRow(ctx, `SELECT k.tenant_id,k.user_id,k.client_id,k.resource,k.scope,k.consent_id,k.session_id,k.challenge,k.redirect_uri,i.provider,i.provider_subject,k.issued_at,k.expires_at,u.created_at,u.expires_at,s.expires_at FROM oauth_codes k JOIN oauth_clients c ON c.client_id=k.client_id JOIN oauth_consents u ON u.tenant_id=k.tenant_id AND u.user_id=k.user_id AND u.consent_id=k.consent_id AND u.client_id=k.client_id AND u.resource=k.resource AND u.scope=k.scope AND u.session_id=k.session_id JOIN oauth_sessions s ON s.tenant_id=k.tenant_id AND s.user_id=k.user_id AND s.session_id=k.session_id JOIN identities i ON i.tenant_id=k.tenant_id AND i.user_id=k.user_id AND i.provider=u.identity_issuer AND i.provider_subject=u.subject WHERE k.code_digest=$1 AND k.consumed_at IS NULL AND c.status='ACTIVE' AND c.client_type='public' AND c.auth_method='none' AND k.redirect_uri=ANY(c.redirect_uris) AND k.scope=ANY(c.scopes) AND k.resource=ANY(c.resources) AND u.revoked_at IS NULL AND NOT EXISTS(SELECT 1 FROM oauth_consent_wallets w WHERE w.tenant_id=u.tenant_id AND w.user_id=u.user_id AND w.consent_id=u.consent_id) AND s.revoked_at IS NULL AND i.status='ACTIVE' FOR UPDATE OF k,c,u,s,i`, r.CodeDigest).Scan(&out.TenantID, &out.UserID, &out.ClientID, &out.Resource, &out.Scope, &out.ConsentID, &out.SessionID, &challenge, &redirect, &out.IdentityIssuer, &out.Subject, &codeIssued, &codeExpires, &consentCreated, &consentExpires, &sessionExpires)
		if e != nil || challenge != r.Challenge || redirect != r.RedirectURI || out.ClientID != r.ClientID || out.Resource != r.Resource || out.Scope != oauth.ReadScope {
			return oauth.ErrDenied
		}
		if e = checkSIWESession(ctx, tx, out.SessionID, out.TenantID, out.UserID); e != nil {
			return e
		}
		// Sample the server-owned wall clock only after all authority row locks
		// are held. PostgreSQL now()/CURRENT_TIMESTAMP would retain transaction
		// start time, and the service timestamp predates any lock wait.
		now := s.now().UTC()
		if now.Before(codeIssued) || now.Before(consentCreated) || !now.Before(codeExpires) || !now.Before(consentExpires) || !now.Before(sessionExpires) {
			return oauth.ErrDenied
		}
		out.Digest = r.TokenDigest
		out.Issuer = oauth.Issuer
		out.IssuedAt = now
		out.ExpiresAt = now.Add(oauth.TokenLifetime)
		for _, v := range []time.Time{consentExpires, sessionExpires} {
			if v.Before(out.ExpiresAt) {
				out.ExpiresAt = v
			}
		}
		_, e = tx.Exec(ctx, `UPDATE oauth_codes SET consumed_at=$2 WHERE code_digest=$1`, r.CodeDigest, now)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO oauth_access_tokens(token_digest,issuer,resource,tenant_id,user_id,client_id,consent_id,session_id,scope,identity_issuer,subject,issued_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, out.Digest, out.Issuer, out.Resource, out.TenantID, out.UserID, out.ClientID, out.ConsentID, out.SessionID, out.Scope, out.IdentityIssuer, out.Subject, out.IssuedAt, out.ExpiresAt)
		if e != nil {
			return e
		}
		return oauthAudit(ctx, tx, "TOKEN_ISSUED", out.TenantID, out.UserID, out.ClientID, out.ConsentID, now)
	})
	return out, e
}
func (s *Store) ValidateOAuthToken(ctx context.Context, digest string, now time.Time) (oauth.Token, error) {
	bounded, cancel, e := s.queryContext(ctx)
	if e != nil {
		return oauth.Token{}, e
	}
	defer cancel()
	t, e := s.queries.ValidateOAuthAccessToken(bounded, dbsqlc.ValidateOAuthAccessTokenParams{TokenDigest: digest, IssuedAt: now})
	if e == nil {
		var invalid bool
		e = s.pool.QueryRow(bounded, `SELECT EXISTS(SELECT 1 FROM siwe_authentications a LEFT JOIN tenants n ON n.tenant_id=a.tenant_id LEFT JOIN wallet_bindings w ON w.tenant_id=a.tenant_id AND w.binding_id=a.binding_id WHERE a.session_reference=$1 AND (a.tenant_id<>$2 OR a.user_id<>$3 OR n.status IS DISTINCT FROM 'ACTIVE' OR w.status IS DISTINCT FROM 'ACTIVE' OR w.version IS DISTINCT FROM a.binding_version OR w.user_id IS DISTINCT FROM a.user_id OR w.revoked_at IS NOT NULL OR w.verified_at IS NULL OR w.verification_reference=''))`, t.SessionID, t.TenantID, t.UserID).Scan(&invalid)
		if invalid {
			e = oauth.ErrDenied
		}
	}
	return oauth.Token{Digest: t.TokenDigest, Issuer: t.Issuer, Resource: t.Resource, ClientID: t.ClientID, TenantID: t.TenantID, UserID: t.UserID, IdentityIssuer: t.IdentityIssuer, Subject: t.Subject, Scope: t.Scope, ConsentID: t.ConsentID, SessionID: t.SessionID, IssuedAt: t.IssuedAt, ExpiresAt: t.ExpiresAt}, e
}
func (s *Store) RevokeOAuthAuthority(ctx context.Context, r oauth.Revocation) error {
	if r.TenantID == "" || r.UserID == "" || r.ID == "" || !slices.Contains([]string{"consent", "session", "token"}, r.Kind) {
		return oauth.ErrDenied
	}
	return s.oauthTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		now := s.now().UTC()
		var client, consent string
		switch r.Kind {
		case "consent":
			e := tx.QueryRow(ctx, `UPDATE oauth_consents SET revoked_at=COALESCE(revoked_at,$4) WHERE tenant_id=$1 AND user_id=$2 AND consent_id=$3 RETURNING client_id,consent_id`, r.TenantID, r.UserID, r.ID, now).Scan(&client, &consent)
			if e != nil {
				return oauth.ErrDenied
			}
		case "token":
			e := tx.QueryRow(ctx, `UPDATE oauth_access_tokens SET revoked_at=COALESCE(revoked_at,$4) WHERE tenant_id=$1 AND user_id=$2 AND token_digest=$3 RETURNING client_id,consent_id`, r.TenantID, r.UserID, r.ID, now).Scan(&client, &consent)
			if e != nil {
				return oauth.ErrDenied
			}
		case "session":
			tag, e := tx.Exec(ctx, `UPDATE oauth_sessions SET revoked_at=COALESCE(revoked_at,$4) WHERE tenant_id=$1 AND user_id=$2 AND session_id=$3`, r.TenantID, r.UserID, r.ID, now)
			if e != nil || tag.RowsAffected() != 1 {
				return oauth.ErrDenied
			}
			_, e = tx.Exec(ctx, `INSERT INTO oauth_audit(event_type,tenant_id,user_id,client_id,consent_id,occurred_at) SELECT 'SESSION_REVOKED',tenant_id,user_id,client_id,consent_id,$4 FROM oauth_consents WHERE tenant_id=$1 AND user_id=$2 AND session_id=$3`, r.TenantID, r.UserID, r.ID, now)
			return e
		}
		event := "CONSENT_REVOKED"
		if r.Kind == "token" {
			event = "TOKEN_REVOKED"
		}
		return oauthAudit(ctx, tx, event, r.TenantID, r.UserID, client, consent, now)
	})
}

var _ oauth.Repository = (*Store)(nil)
