package postgres

import (
	"context"
	"crypto/subtle"
	"time"

	"github.com/deseti/wizpay-mcp/internal/browser"
	"github.com/deseti/wizpay-mcp/internal/siwe"
	"github.com/jackc/pgx/v5"
)

func siwePending(v browser.Session, r siwe.Request) bool {
	return v.State == "PENDING" && v.Decision == "OPEN" && !v.Revoked && subtle.ConstantTimeCompare([]byte(v.CSRFHash), []byte(r.CSRFDigest)) == 1
}
func dbWallTime(ctx context.Context, tx pgx.Tx) (time.Time, error) {
	var now time.Time
	e := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
	return now, e
}
func (s *Store) IssueSIWE(ctx context.Context, r siwe.Request) (siwe.Challenge, error) {
	var c siwe.Challenge
	e := s.oauthTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		v, e := s.lockBrowser(ctx, tx, "session_digest", r.SessionDigest)
		if e != nil || !siwePending(v, r) {
			return siwe.ErrDenied
		}
		expiry, e := s.lockBrowserTransaction(ctx, tx, v.TransactionID)
		if e != nil {
			return siwe.ErrDenied
		}
		var tenant string
		if e = tx.QueryRow(ctx, `SELECT tenant_id FROM tenants WHERE tenant_id=$1 AND status='ACTIVE' FOR UPDATE`, r.TenantID).Scan(&tenant); e != nil {
			return siwe.ErrDenied
		}
		var count int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM siwe_challenges WHERE session_reference=$1`, v.Reference).Scan(&count); e != nil {
			return e
		}
		if count >= 3 {
			return siwe.ErrDenied
		}
		now, e := dbWallTime(ctx, tx)
		if e != nil {
			return e
		}
		if !now.Before(v.ExpiresAt) || !now.Before(expiry) {
			return siwe.ErrDenied
		}
		c = siwe.Challenge{Address: r.Address, SessionReference: v.Reference, TransactionID: v.TransactionID, TenantID: r.TenantID, IssuedAt: now, ExpiresAt: now.Add(siwe.Lifetime)}
		if expiry.Before(c.ExpiresAt) {
			c.ExpiresAt = expiry
		}
		if v.ExpiresAt.Before(c.ExpiresAt) {
			c.ExpiresAt = v.ExpiresAt
		}
		if c.ID, e = siwe.Random(); e != nil {
			return e
		}
		if c.Nonce, e = siwe.Random(); e != nil {
			return e
		}
		if c.Message, e = siwe.Message(c); e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, `SELECT client_id FROM oauth_transactions WHERE transaction_id=$1`, v.TransactionID).Scan(&c.ClientID); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO siwe_challenges(challenge_id,nonce,message,address,tenant_id,session_reference,transaction_id,client_id,domain,uri,chain_id,issued_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'connect.wizpay.xyz','https://connect.wizpay.xyz','5042',$9,$10)`, c.ID, c.Nonce, c.Message, c.Address, c.TenantID, c.SessionReference, c.TransactionID, c.ClientID, c.IssuedAt, c.ExpiresAt)
		return e
	})
	return c, e
}
func (s *Store) FinalizeSIWE(ctx context.Context, r siwe.Request, id, signature string, next siwe.Rotation) error {
	return s.oauthTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// Browser first matches consent/logout lock order. Challenge is locked before
		// any consumption; no other path acquires challenge then browser.
		v, e := s.lockBrowser(ctx, tx, "session_digest", r.SessionDigest)
		if e != nil || !siwePending(v, r) {
			return siwe.ErrDenied
		}
		var c siwe.Challenge
		e = tx.QueryRow(ctx, `SELECT challenge_id,nonce,message,address,session_reference,transaction_id,client_id,tenant_id,issued_at,expires_at FROM siwe_challenges WHERE challenge_id=$1 AND consumed_at IS NULL FOR UPDATE`, id).Scan(&c.ID, &c.Nonce, &c.Message, &c.Address, &c.SessionReference, &c.TransactionID, &c.ClientID, &c.TenantID, &c.IssuedAt, &c.ExpiresAt)
		if e != nil || c.SessionReference != v.Reference || c.TransactionID != v.TransactionID || c.TenantID != r.TenantID {
			return siwe.ErrDenied
		}
		expiry, e := s.lockBrowserTransaction(ctx, tx, v.TransactionID)
		if e != nil {
			return siwe.ErrDenied
		}
		var client string
		if e = tx.QueryRow(ctx, `SELECT client_id FROM oauth_transactions WHERE transaction_id=$1`, v.TransactionID).Scan(&client); e != nil || client != c.ClientID {
			return siwe.ErrDenied
		}
		// Resolve an existing owner without granting authority, then lock identity
		// before tenant/binding to match OAuth redemption lock ordering. The
		// authoritative binding match is repeated under locks below.
		var resolvedUser, issuer, subject string
		var matches int
		if e = tx.QueryRow(ctx, `SELECT COALESCE(min(user_id),''),count(*) FROM wallet_bindings WHERE tenant_id=$1 AND lower(address)=lower($2) AND chain_id='5042' AND network='MAINNET'`, r.TenantID, c.Address).Scan(&resolvedUser, &matches); e != nil || matches != 1 {
			return siwe.ErrDenied
		}
		if e = tx.QueryRow(ctx, `SELECT provider,provider_subject FROM identities WHERE tenant_id=$1 AND user_id=$2 AND status='ACTIVE' FOR UPDATE`, r.TenantID, resolvedUser).Scan(&issuer, &subject); e != nil {
			return siwe.ErrDenied
		}
		var tenant string
		if e = tx.QueryRow(ctx, `SELECT tenant_id FROM tenants WHERE tenant_id=$1 AND status='ACTIVE' FOR UPDATE`, r.TenantID).Scan(&tenant); e != nil {
			return siwe.ErrDenied
		}
		// Lock ALL case-equivalent matches, including inactive ones. Duplicates are
		// identity conflicts, never filtered down to a preferred ACTIVE candidate.
		rows, e := tx.Query(ctx, `SELECT binding_id,version,user_id FROM wallet_bindings WHERE tenant_id=$1 AND lower(address)=lower($2) AND chain_id='5042' AND network='MAINNET' ORDER BY binding_id FOR UPDATE`, r.TenantID, c.Address)
		if e != nil {
			return e
		}
		var binding, user string
		var version int64
		count := 0
		for rows.Next() {
			count++
			if e = rows.Scan(&binding, &version, &user); e != nil {
				rows.Close()
				return e
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if count != 1 {
			return siwe.ErrDenied
		}
		var eligible bool
		if e = tx.QueryRow(ctx, `SELECT provider='EXTERNAL_EVM' AND status='ACTIVE' AND revoked_at IS NULL AND verified_at IS NOT NULL AND verified_at<=clock_timestamp() AND verification_reference<>'' FROM wallet_bindings WHERE tenant_id=$1 AND binding_id=$2`, r.TenantID, binding).Scan(&eligible); e != nil || !eligible {
			return siwe.ErrDenied
		}
		if user != resolvedUser {
			return siwe.ErrDenied
		}
		now, e := dbWallTime(ctx, tx)
		if e != nil {
			return e
		}
		if !siwe.ValidTime(c, now) || !now.Before(v.ExpiresAt) || !now.Before(expiry) {
			return siwe.ErrDenied
		}
		// Stored bytes are the signing authority; formatter is never used here.
		if e = siwe.Verify(c.Message, c.Address, signature); e != nil {
			return e
		}
		now, e = dbWallTime(ctx, tx)
		if e != nil {
			return e
		}
		if !siwe.ValidTime(c, now) || !now.Before(v.ExpiresAt) || !now.Before(expiry) {
			return siwe.ErrDenied
		}
		if next.Digest == v.Digest || next.Reference == v.Reference || next.Digest == "" || next.CSRFHash == "" || next.Reference == "" {
			return siwe.ErrDenied
		}
		if _, e = tx.Exec(ctx, `UPDATE siwe_challenges SET consumed_at=$2 WHERE challenge_id=$1`, id, now); e != nil {
			return e
		}
		if e = revokeBrowser(ctx, tx, v, now); e != nil {
			return e
		}
		n := browser.Session{Digest: next.Digest, CSRFHash: next.CSRFHash, Reference: next.Reference, TransactionID: v.TransactionID, State: "AUTHENTICATED", Decision: "OPEN", TenantID: r.TenantID, UserID: user, Issuer: issuer, Subject: subject, Evidence: "siwe:" + id, CreatedAt: now, ExpiresAt: now.Add(browser.SessionLifetime)}
		if e = insertBrowser(ctx, tx, n); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO siwe_authentications(session_reference,challenge_id,tenant_id,user_id,binding_id,binding_version,authenticated_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, n.Reference, id, r.TenantID, user, binding, version, now)
		return e
	})
}

var _ siwe.Repository = (*Store)(nil)

// SIWE-derived sessions retain their exact binding version. Legacy browser
// fixtures have no SIWE record and retain the existing WP3A authority checks.
func checkSIWESession(ctx context.Context, tx pgx.Tx, reference, tenant, user string) error {
	var evidence bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM siwe_authentications WHERE session_reference=$1)`, reference).Scan(&evidence); e != nil {
		return e
	}
	if !evidence {
		return nil
	}
	var valid bool
	e := tx.QueryRow(ctx, `SELECT t.status='ACTIVE' AND b.status='ACTIVE' AND b.provider='EXTERNAL_EVM' AND b.chain_id='5042' AND b.network='MAINNET' AND b.revoked_at IS NULL AND b.verified_at IS NOT NULL AND b.verification_reference<>'' AND b.version=a.binding_version AND b.user_id=a.user_id FROM siwe_authentications a JOIN tenants t ON t.tenant_id=a.tenant_id JOIN wallet_bindings b ON b.tenant_id=a.tenant_id AND b.binding_id=a.binding_id WHERE a.session_reference=$1 AND a.tenant_id=$2 AND a.user_id=$3 FOR SHARE OF t,b`, reference, tenant, user).Scan(&valid)
	if e != nil || !valid {
		return siwe.ErrDenied
	}
	return nil
}
