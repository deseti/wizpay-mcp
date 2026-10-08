package postgres

import (
	"context"
	"github.com/deseti/wizpay-mcp/internal/browser"
	"github.com/jackc/pgx/v5"
	"time"
)

const browserColumns = `session_digest,csrf_digest,session_reference,transaction_id,state,decision,COALESCE(tenant_id,''),COALESCE(user_id,''),COALESCE(identity_issuer,''),COALESCE(subject,''),COALESCE(evidence_reference,''),created_at,expires_at,revoked_at IS NOT NULL`

func scanBrowser(row pgx.Row) (browser.Session, error) {
	var v browser.Session
	e := row.Scan(&v.Digest, &v.CSRFHash, &v.Reference, &v.TransactionID, &v.State, &v.Decision, &v.TenantID, &v.UserID, &v.Issuer, &v.Subject, &v.Evidence, &v.CreatedAt, &v.ExpiresAt, &v.Revoked)
	return v, e
}
func insertBrowser(ctx context.Context, tx pgx.Tx, v browser.Session) error {
	_, e := tx.Exec(ctx, `INSERT INTO browser_sessions(session_digest,csrf_digest,session_reference,transaction_id,state,decision,tenant_id,user_id,identity_issuer,subject,evidence_reference,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),$12,$13)`, v.Digest, v.CSRFHash, v.Reference, v.TransactionID, v.State, v.Decision, v.TenantID, v.UserID, v.Issuer, v.Subject, v.Evidence, v.CreatedAt, v.ExpiresAt)
	return e
}
func (s *Store) CreateBrowserSession(ctx context.Context, v browser.Session) error {
	if v.State != "PENDING" || v.TenantID != "" || v.UserID != "" || v.Decision != "OPEN" {
		return browser.ErrDenied
	}
	return s.oauthTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		expiry, e := s.lockBrowserTransaction(ctx, tx, v.TransactionID)
		if e != nil {
			return e
		}
		now := s.now().UTC()
		if !now.Before(expiry) || !now.Before(v.ExpiresAt) || now.Before(v.CreatedAt) || v.ExpiresAt.After(expiry) {
			return browser.ErrDenied
		}
		return insertBrowser(ctx, tx, v)
	})
}
func (s *Store) FindBrowserSession(ctx context.Context, digest string) (browser.Session, error) {
	b, c, e := s.queryContext(ctx)
	if e != nil {
		return browser.Session{}, e
	}
	defer c()
	return scanBrowser(s.pool.QueryRow(b, `SELECT `+browserColumns+` FROM browser_sessions WHERE session_digest=$1`, digest))
}
func (s *Store) lockBrowser(ctx context.Context, tx pgx.Tx, column, key string) (browser.Session, error) {
	// column is selected only by internal callers, never request input.
	v, e := scanBrowser(tx.QueryRow(ctx, `SELECT `+browserColumns+` FROM browser_sessions WHERE `+column+`=$1 FOR UPDATE`, key))
	now := s.now().UTC()
	if e != nil || v.Revoked || !now.Before(v.ExpiresAt) || now.Before(v.CreatedAt) {
		return browser.Session{}, browser.ErrDenied
	}
	return v, nil
}
func (s *Store) lockBrowserTransaction(ctx context.Context, tx pgx.Tx, id string) (time.Time, error) {
	var expiry time.Time
	e := tx.QueryRow(ctx, `SELECT t.expires_at FROM oauth_transactions t JOIN oauth_clients c ON c.client_id=t.client_id WHERE t.transaction_id=$1 AND t.completed_at IS NULL AND c.status='ACTIVE' AND t.redirect_uri=ANY(c.redirect_uris) AND t.resource=ANY(c.resources) AND t.scope=ANY(c.scopes) FOR UPDATE OF t,c`, id).Scan(&expiry)
	if e != nil {
		return expiry, browser.ErrDenied
	}
	return expiry, nil
}
func revokeBrowser(ctx context.Context, tx pgx.Tx, v browser.Session, now time.Time) error {
	if _, e := tx.Exec(ctx, `UPDATE browser_sessions SET revoked_at=$2 WHERE session_digest=$1`, v.Digest, now); e != nil {
		return e
	}
	if v.State != "AUTHENTICATED" {
		return nil
	}
	_, e := tx.Exec(ctx, `UPDATE oauth_sessions SET revoked_at=COALESCE(revoked_at,$2) WHERE session_id=$1 AND tenant_id=$3 AND user_id=$4`, v.Reference, now, v.TenantID, v.UserID)
	return e
}
func (s *Store) RotateBrowserSession(ctx context.Context, digest, proof string, next browser.Session) error {
	return s.oauthTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		old, e := s.lockBrowser(ctx, tx, "session_digest", digest)
		if e != nil || old.CSRFHash != proof || old.Decision != "OPEN" || next.TransactionID != old.TransactionID || next.State != "AUTHENTICATED" || next.Decision != "OPEN" || next.Reference == old.Reference || next.Digest == old.Digest {
			return browser.ErrDenied
		}
		if old.State == "AUTHENTICATED" && (old.TenantID != next.TenantID || old.UserID != next.UserID || old.Issuer != next.Issuer || old.Subject != next.Subject) {
			return browser.ErrDenied
		}
		expiry, e := s.lockBrowserTransaction(ctx, tx, old.TransactionID)
		if e != nil {
			return e
		}
		var user string
		e = tx.QueryRow(ctx, `SELECT user_id FROM identities WHERE tenant_id=$1 AND user_id=$2 AND provider=$3 AND provider_subject=$4 AND status='ACTIVE' FOR UPDATE`, next.TenantID, next.UserID, next.Issuer, next.Subject).Scan(&user)
		if e != nil {
			return browser.ErrDenied
		}
		now := s.now().UTC()
		if !now.Before(old.ExpiresAt) || !now.Before(expiry) || !now.Before(next.ExpiresAt) || now.Before(next.CreatedAt) {
			return browser.ErrDenied
		}
		if e = revokeBrowser(ctx, tx, old, now); e != nil {
			return e
		}
		return insertBrowser(ctx, tx, next)
	})
}
func (s *Store) RevokeBrowserSession(ctx context.Context, digest, proof string) error {
	return s.oauthTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		v, e := s.lockBrowser(ctx, tx, "session_digest", digest)
		if e != nil || v.CSRFHash != proof {
			return browser.ErrDenied
		}
		return revokeBrowser(ctx, tx, v, s.now().UTC())
	})
}
func (s *Store) DenyBrowserConsent(ctx context.Context, digest, proof string) error {
	return s.oauthTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		v, e := s.lockBrowser(ctx, tx, "session_digest", digest)
		if e != nil || v.CSRFHash != proof || v.Decision != "OPEN" {
			return browser.ErrDenied
		}
		expiry, e := s.lockBrowserTransaction(ctx, tx, v.TransactionID)
		if e != nil {
			return e
		}
		now := s.now().UTC()
		if !now.Before(v.ExpiresAt) || !now.Before(expiry) {
			return browser.ErrDenied
		}
		if _, e = tx.Exec(ctx, `UPDATE oauth_transactions SET completed_at=$2 WHERE transaction_id=$1`, v.TransactionID, now); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE browser_sessions SET decision='DENIED' WHERE session_digest=$1`, digest)
		return e
	})
}

// Only anonymous tombstones are deleted. Authenticated references are retained
// for revocation/audit relationships; WP3B must supply an approved retention policy.
func (s *Store) CleanupBrowserSessions(ctx context.Context) error {
	b, c, e := s.queryContext(ctx)
	if e != nil {
		return e
	}
	defer c()
	_, e = s.pool.Exec(b, `DELETE FROM browser_sessions WHERE state='PENDING' AND expires_at<clock_timestamp()-interval '24 hours'`)
	return e
}

var _ browser.Repository = (*Store)(nil)
