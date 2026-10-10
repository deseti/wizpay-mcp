package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/oauth"
)

func TestProtocolAuthorityRevocation(t *testing.T) {
	for _, kind := range []string{"token", "consent", "session", "expired-token"} {
		t.Run(kind, func(t *testing.T) {
			f := newProtocolFixture(t, "5042")
			login := f.login(t)
			token := f.redeem(t, f.grant(t, login))
			ctx := context.Background()
			authority, err := f.store.ValidateOAuthToken(ctx, oauth.Digest(token), f.store.now())
			if err != nil {
				t.Fatal("fresh token rejected")
			}
			protocolStatus(t, f.mcp(t, token, "tools/list", map[string]any{}), 200)
			if kind == "expired-token" {
				f.offset.Store(int64(time.Until(authority.ExpiresAt) + time.Second))
			} else {
				id := authority.Digest
				switch kind {
				case "consent":
					id = authority.ConsentID
				case "session":
					id = authority.SessionID
				}
				// Synthetic control-plane revocation via the existing persistence port.
				// No new public endpoint or fabricated browser revocation principal.
				err = f.store.RevokeOAuthAuthority(ctx, oauth.Revocation{TenantID: f.base.scope.TenantID(), UserID: f.base.scope.ActorID(), ID: id, Kind: kind})
				if err != nil {
					t.Fatal("revocation failed")
				}
			}
			protocolStatus(t, f.mcp(t, token, "tools/list", map[string]any{}), 401)
			var browserRevoked, sessionRevoked, consentRevoked, tokenRevoked bool
			err = integrationPool.QueryRow(ctx, `SELECT b.revoked_at IS NOT NULL,s.revoked_at IS NOT NULL,c.revoked_at IS NOT NULL,a.revoked_at IS NOT NULL FROM oauth_access_tokens a JOIN oauth_sessions s ON s.session_id=a.session_id JOIN oauth_consents c ON c.consent_id=a.consent_id JOIN browser_sessions b ON b.session_reference=a.session_id WHERE a.token_digest=$1`, oauth.Digest(token)).Scan(&browserRevoked, &sessionRevoked, &consentRevoked, &tokenRevoked)
			if err != nil || browserRevoked || sessionRevoked != (kind == "session") || consentRevoked != (kind == "consent") || tokenRevoked != (kind == "token") {
				t.Fatal("independent revocation semantics changed")
			}
		})
	}
}

func TestProtocolRevokedConsentBlocksCodeRedemption(t *testing.T) {
	f := newProtocolFixture(t, "5042")
	login := f.login(t)
	code := f.grant(t, login)
	ctx := context.Background()
	var consent string
	if err := integrationPool.QueryRow(ctx, `SELECT consent_id FROM oauth_codes WHERE code_digest=$1`, oauth.Digest(code)).Scan(&consent); err != nil {
		t.Fatal(err)
	}
	if err := f.store.RevokeOAuthAuthority(ctx, oauth.Revocation{TenantID: f.base.scope.TenantID(), UserID: f.base.scope.ActorID(), ID: consent, Kind: "consent"}); err != nil {
		t.Fatal(err)
	}
	protocolStatus(t, f.browser("POST", "/oauth/token", f.exchange(code).Encode(), nil, ""), 400)
	var consumed bool
	var tokens int
	err := integrationPool.QueryRow(ctx, `SELECT consumed_at IS NOT NULL,(SELECT count(*) FROM oauth_access_tokens WHERE consent_id=$2) FROM oauth_codes WHERE code_digest=$1`, oauth.Digest(code), consent).Scan(&consumed, &tokens)
	if err != nil || consumed || tokens != 0 {
		t.Fatal("revoked consent issued credentials or consumed code")
	}
}
