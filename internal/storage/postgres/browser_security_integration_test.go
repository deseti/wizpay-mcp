package postgres

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/browser"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/jackc/pgx/v5"
)

func authenticatedBrowser(t *testing.T) (*browser.Service, *Store, string, browser.View, oauth.Transaction) {
	t.Helper()
	s, store, raw, tr := browserFixture(t)
	v, err := s.View(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = s.Authenticate(context.Background(), raw, v.CSRF)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.View(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	return s, store, raw, v, tr
}

func TestBrowserConcurrentRotation(t *testing.T) {
	s, store, raw, tr := browserFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	v, err := s.View(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := s.Authenticate(ctx, raw, v.CSRF); err == nil {
				successes.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	var active int
	if err := integrationPool.QueryRow(ctx, `SELECT count(*) FROM browser_sessions WHERE transaction_id=$1 AND state='AUTHENTICATED' AND revoked_at IS NULL`, tr.ID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if successes.Load() != 1 || active != 1 {
		t.Fatalf("rotation successes=%d active=%d", successes.Load(), active)
	}
	if _, err = store.FindBrowserSession(ctx, oauth.Digest(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.View(ctx, raw); err == nil {
		t.Fatal("old credential regained authority")
	}
}

func TestBrowserGrantCompetingWithTerminalAction(t *testing.T) {
	for _, action := range []string{"logout", "deny"} {
		t.Run(action, func(t *testing.T) {
			s, store, raw, v, tr := authenticatedBrowser(t)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			start := make(chan struct{})
			grant := make(chan struct {
				redirect string
				err      error
			}, 1)
			terminal := make(chan error, 1)
			go func() {
				<-start
				redirect, err := s.Grant(ctx, raw, v.CSRF)
				grant <- struct {
					redirect string
					err      error
				}{redirect, err}
			}()
			go func() {
				<-start
				if action == "logout" {
					terminal <- s.Logout(ctx, raw, v.CSRF)
				} else {
					terminal <- s.Deny(ctx, raw, v.CSRF)
				}
			}()
			close(start)
			g := <-grant
			terminalErr := <-terminal
			var codes int
			if err := integrationPool.QueryRow(ctx, `SELECT count(*) FROM oauth_codes WHERE transaction_id=$1`, tr.ID).Scan(&codes); err != nil {
				t.Fatal(err)
			}
			if g.err == nil && codes != 1 || g.err != nil && codes != 0 {
				t.Fatal("non-atomic grant")
			}
			if action == "deny" {
				if (g.err == nil) == (terminalErr == nil) {
					t.Fatal("grant and denial did not serialize to one winner")
				}
			} else {
				if terminalErr != nil {
					t.Fatal(terminalErr)
				}
				if _, err := s.Grant(ctx, raw, v.CSRF); err == nil {
					t.Fatal("revoked browser granted")
				}
				if g.err == nil {
					u, err := url.Parse(g.redirect)
					if err != nil {
						t.Fatal(err)
					}
					flow, err := oauth.NewService(store, store.now)
					if err != nil {
						t.Fatal(err)
					}
					if _, err = flow.Exchange(ctx, oauth.Exchange{GrantType: "authorization_code", ClientID: tr.Request.ClientID, RedirectURI: tr.Request.RedirectURI, Resource: tr.Request.Resource, Code: u.Query().Get("code"), Verifier: oauthVerifier}); err == nil {
						t.Fatal("logged-out session redeemed code")
					}
				}
			}
		})
	}
}

// Observe an actual PostgreSQL lock wait before advancing the repository clock.
func waitBrowserBlock(t *testing.T, ctx context.Context, pid uint32) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := integrationPool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::int=ANY(pg_blocking_pids(pid)))`, int32(pid)).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("authority operation never reached lock wait")
		case <-ticker.C:
		}
	}
}
func TestBrowserConsentCompletionExpiryAcrossLockWait(t *testing.T) {
	for _, kind := range []string{"transaction", "browser"} {
		t.Run(kind, func(t *testing.T) {
			s, store, raw, v, tr := authenticatedBrowser(t)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			record, err := store.FindBrowserSession(ctx, oauth.Digest(raw))
			if err != nil {
				t.Fatal(err)
			}
			if kind == "browser" {
				// Expire the browser independently while the OAuth transaction is still live.
				p, err := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: record.TenantID, ActorID: record.UserID, IdentityProvider: record.Issuer, ProviderSubject: record.Subject, ExpiresAt: record.CreatedAt.Add(time.Minute)})
				if err != nil {
					t.Fatal(err)
				}
				flow, err := oauth.NewService(store, store.now)
				if err != nil {
					t.Fatal(err)
				}
				s, err = browser.NewService(store, store, flow, shortBrowserVerifier{p}, store.now)
				if err != nil {
					t.Fatal(err)
				}
				raw, err = s.Authenticate(ctx, raw, v.CSRF)
				if err != nil {
					t.Fatal(err)
				}
				v, err = s.View(ctx, raw)
				if err != nil {
					t.Fatal(err)
				}
				record, err = store.FindBrowserSession(ctx, oauth.Digest(raw))
				if err != nil {
					t.Fatal(err)
				}
				if !record.ExpiresAt.Before(tr.ExpiresAt) {
					t.Fatal("browser expiry not isolated")
				}
			}
			var clock atomic.Int64
			clock.Store(record.CreatedAt.UnixNano())
			store.now = func() time.Time { return time.Unix(0, clock.Load()).UTC() }
			blocker, err := integrationPool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback(context.Background())
			if _, err = blocker.Exec(ctx, `SELECT client_id FROM oauth_clients WHERE client_id=$1 FOR UPDATE`, tr.Request.ClientID); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { _, err := s.Grant(ctx, raw, v.CSRF); result <- err }()
			waitBrowserBlock(t, ctx, blocker.Conn().PgConn().PID())
			expiry := tr.ExpiresAt
			if kind == "browser" {
				expiry = record.ExpiresAt
			}
			clock.Store(expiry.Add(time.Second).UnixNano())
			if err = blocker.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-result; err == nil {
				t.Fatal("expired authority issued code")
			}
			assertBrowserCompletionRolledBack(t, ctx, tr, record)
		})
	}
}
func assertBrowserCompletionRolledBack(t *testing.T, ctx context.Context, tr oauth.Transaction, record browser.Session, expectedSessions ...int) {
	t.Helper()
	var codes, consents, sessions, audits int
	var decision string
	var completed bool
	err := integrationPool.QueryRow(ctx, `SELECT (SELECT count(*) FROM oauth_codes WHERE transaction_id=$1),(SELECT count(*) FROM oauth_consents WHERE session_id=$2),(SELECT count(*) FROM oauth_sessions WHERE session_id=$2),(SELECT count(*) FROM oauth_audit WHERE client_id=$3 AND event_type='AUTHORIZED'),(SELECT decision FROM browser_sessions WHERE session_reference=$2),(SELECT completed_at IS NOT NULL FROM oauth_transactions WHERE transaction_id=$1)`, tr.ID, record.Reference, tr.Request.ClientID).Scan(&codes, &consents, &sessions, &audits, &decision, &completed)
	// browserFixture also creates a separate legacy authorization for this client.
	if err != nil {
		t.Fatal(err)
	}
	wantSessions := 0
	if len(expectedSessions) > 0 {
		wantSessions = expectedSessions[0]
	}
	if codes != 0 || consents != 0 || sessions != wantSessions || audits != 1 || decision != "OPEN" || completed {
		t.Fatalf("partial issuance: codes=%d consents=%d sessions=%d audits=%d decision=%s completed=%v", codes, consents, sessions, audits, decision, completed)
	}
}
func TestBrowserConsentAtomicRollback(t *testing.T) {
	for _, table := range []string{"oauth_consents", "oauth_codes", "oauth_audit"} {
		t.Run(table, func(t *testing.T) {
			s, store, raw, v, tr := authenticatedBrowser(t)
			ctx := context.Background()
			record, err := store.FindBrowserSession(ctx, oauth.Digest(raw))
			if err != nil {
				t.Fatal(err)
			}
			name := unique("browser_fault")
			fn := pgx.Identifier{name}.Sanitize()
			trigger := pgx.Identifier{name + "_trigger"}.Sanitize()
			tbl := pgx.Identifier{table}.Sanitize()
			if _, err = integrationPool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF to_jsonb(NEW)->>'client_id'=TG_ARGV[0] THEN RAISE EXCEPTION 'isolated test fault'; END IF; RETURN NEW; END $$`, fn)); err != nil {
				t.Fatal(err)
			}
			defer integrationPool.Exec(ctx, "DROP FUNCTION IF EXISTS "+fn+"() CASCADE")
			// Client identifiers are quoted as SQL literals only in this isolated fixture.
			arg := fmt.Sprintf("'%s'", tr.Request.ClientID)
			if _, err = integrationPool.Exec(ctx, fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION %s(%s)`, trigger, tbl, fn, arg)); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Grant(ctx, raw, v.CSRF); err == nil {
				t.Fatal("fault did not abort issuance")
			}
			assertBrowserCompletionRolledBack(t, ctx, tr, record)
			if _, err = integrationPool.Exec(ctx, "DROP TRIGGER "+trigger+" ON "+tbl); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Grant(ctx, raw, v.CSRF); err != nil {
				t.Fatal("rollback lost eligible authority", err)
			}
		})
	}
}

// This trusted verifier exists only in tests; no HTTP route accepts its output.
type shortBrowserVerifier struct{ p auth.AuthenticatedPrincipal }

func (v shortBrowserVerifier) VerifyBrowser(_ context.Context, r browser.VerificationRequest) (browser.Verification, error) {
	return browser.Verification{Principal: v.p, SessionReference: r.SessionReference, TransactionID: r.TransactionID, Evidence: "isolated-short-lived-fixture"}, nil
}

func TestBrowserCompletionIdentityAndClientIsolation(t *testing.T) {
	for _, kind := range []string{"user", "tenant", "client", "transaction"} {
		t.Run(kind, func(t *testing.T) {
			_, store, raw, _, tr := authenticatedBrowser(t)
			ctx := context.Background()
			record, err := store.FindBrowserSession(ctx, oauth.Digest(raw))
			if err != nil {
				t.Fatal(err)
			}
			p, err := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: record.TenantID, ActorID: record.UserID, IdentityProvider: record.Issuer, ProviderSubject: record.Subject, ExpiresAt: record.ExpiresAt})
			if err != nil {
				t.Fatal(err)
			}
			d := oauth.BrowserDecision{Principal: p, SessionID: record.Reference, SessionExpiresAt: record.ExpiresAt, TransactionID: tr.ID, ClientID: tr.Request.ClientID, Resource: tr.Request.Resource, Scope: tr.Request.Scope, BrowserSessionReference: record.Reference}
			k := oauth.Code{Digest: oauth.Digest(unique("test-code")), TransactionID: tr.ID, ConsentID: unique("test-consent"), SessionID: record.Reference, TenantID: record.TenantID, UserID: record.UserID, IssuedAt: record.CreatedAt, ExpiresAt: record.CreatedAt.Add(time.Minute)}
			other := newOAuthFixture(t) // Real, isolated identities and registered client.
			altered := tr
			switch kind {
			case "user":
				k.UserID = other.f.scope.ActorID()
			case "tenant":
				k.TenantID = other.f.scope.TenantID()
			case "client":
				altered.Request.ClientID = other.request.ClientID
			case "transaction":
				altered.ID = other.transaction.ID
			}
			if err = store.CompleteOAuthAuthorization(ctx, altered, d, k); err == nil {
				t.Fatal("cross-authority completion accepted")
			}
			assertBrowserCompletionRolledBack(t, ctx, tr, record)
		})
	}
}
func TestBrowserConsentRevocationIsTerminal(t *testing.T) {
	s, store, raw, v, tr := authenticatedBrowser(t)
	ctx := context.Background()
	redirect, err := s.Grant(ctx, raw, v.CSRF)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(redirect)
	if err != nil {
		t.Fatal(err)
	}
	var tenant, user, consent string
	if err = integrationPool.QueryRow(ctx, `SELECT tenant_id,user_id,consent_id FROM oauth_codes WHERE transaction_id=$1`, tr.ID).Scan(&tenant, &user, &consent); err != nil {
		t.Fatal(err)
	}
	if err = store.RevokeOAuthAuthority(ctx, oauth.Revocation{TenantID: tenant, UserID: user, Kind: "consent", ID: consent}); err != nil {
		t.Fatal(err)
	}
	flow, err := oauth.NewService(store, store.now)
	if err != nil {
		t.Fatal(err)
	}
	x := oauth.Exchange{GrantType: "authorization_code", ClientID: tr.Request.ClientID, RedirectURI: tr.Request.RedirectURI, Resource: tr.Request.Resource, Code: u.Query().Get("code"), Verifier: oauthVerifier}
	for i := 0; i < 2; i++ {
		if _, err = flow.Exchange(ctx, x); err == nil {
			t.Fatal("revoked consent regained authority")
		}
	}
	if _, err = integrationPool.Exec(ctx, `UPDATE oauth_consents SET revoked_at=NULL WHERE consent_id=$1`, consent); err == nil {
		t.Fatal("consent revocation cleared")
	}
	if _, err = s.Grant(ctx, raw, v.CSRF); err == nil {
		t.Fatal("consumed browser reissued authority")
	}
}

func TestBrowserOAuthSessionExpiryDuringCompletion(t *testing.T) {
	s, store, raw, v, tr := authenticatedBrowser(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	record, err := store.FindBrowserSession(ctx, oauth.Digest(raw))
	if err != nil {
		t.Fatal(err)
	}
	expiry := record.CreatedAt.Add(time.Minute)
	if _, err = integrationPool.Exec(ctx, `INSERT INTO oauth_sessions(tenant_id,user_id,session_id,expires_at) VALUES($1,$2,$3,$4)`, record.TenantID, record.UserID, record.Reference, expiry); err != nil {
		t.Fatal(err)
	}
	var clock atomic.Int64
	clock.Store(record.CreatedAt.UnixNano())
	store.now = func() time.Time { return time.Unix(0, clock.Load()).UTC() }
	blocker, err := integrationPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err = blocker.Exec(ctx, `SELECT session_id FROM oauth_sessions WHERE session_id=$1 FOR UPDATE`, record.Reference); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := s.Grant(ctx, raw, v.CSRF); result <- err }()
	waitBrowserBlock(t, ctx, blocker.Conn().PgConn().PID())
	clock.Store(expiry.Add(time.Second).UnixNano())
	if err = blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; err == nil {
		t.Fatal("expired OAuth session issued browser authorization")
	}
	assertBrowserCompletionRolledBack(t, ctx, tr, record, 1)
}
