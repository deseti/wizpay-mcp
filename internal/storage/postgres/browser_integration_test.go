package postgres

import (
	"context"
	"github.com/deseti/wizpay-mcp/internal/browser"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type browserVerifierFixture struct{ f oauthFixture }

func (v browserVerifierFixture) VerifyBrowser(_ context.Context, r browser.VerificationRequest) (browser.Verification, error) {
	return browser.Verification{Principal: v.f.browser.principal, SessionReference: r.SessionReference, TransactionID: r.TransactionID, Evidence: "test-only-verified-evidence"}, nil
}
func browserFixture(t *testing.T) (*browser.Service, *Store, string, oauth.Transaction) {
	t.Helper()
	f := newOAuthFixture(t)
	store := *integrationStore
	store.now = func() time.Time { return *f.now }
	flow, e := oauth.NewService(&store, store.now)
	if e != nil {
		t.Fatal(e)
	}
	tr, e := flow.Begin(context.Background(), f.request)
	if e != nil {
		t.Fatal(e)
	}
	s, e := browser.NewService(&store, &store, flow, browserVerifierFixture{f}, store.now)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := s.Start(context.Background(), tr)
	if e != nil {
		t.Fatal(e)
	}
	return s, &store, raw, tr
}
func TestBrowserRotationGrantAndConcurrentReplay(t *testing.T) {
	ctx := context.Background()
	s, store, raw, tr := browserFixture(t)
	view, e := s.View(ctx, raw)
	if e != nil {
		t.Fatal(e)
	}
	next, e := s.Authenticate(ctx, raw, view.CSRF)
	if e != nil {
		t.Fatal(e)
	}
	if next == raw {
		t.Fatal("session fixation")
	}
	if _, e = s.View(ctx, raw); e == nil {
		t.Fatal("old session alive")
	}
	view, e = s.View(ctx, next)
	if e != nil || view.State != "AUTHENTICATED" || view.UserID == "" {
		t.Fatal("verified session unavailable")
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.Grant(ctx, next, view.CSRF); e == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("grant successes=%d", successes.Load())
	}
	var codes int
	if e = integrationPool.QueryRow(ctx, `SELECT count(*) FROM oauth_codes WHERE transaction_id=$1`, tr.ID).Scan(&codes); e != nil || codes != 1 {
		t.Fatal("duplicate code")
	}
	if e = s.Logout(ctx, next, view.CSRF); e != nil {
		t.Fatal(e)
	}
	var revoked bool
	if e = integrationPool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM oauth_sessions WHERE session_id=(SELECT session_reference FROM browser_sessions WHERE session_digest=$1)`, oauth.Digest(next)).Scan(&revoked); e != nil || !revoked {
		t.Fatal("OAuth session survived logout")
	}
	_ = store
}
func TestBrowserPendingDenyAndIsolation(t *testing.T) {
	ctx := context.Background()
	s, store, raw, tr := browserFixture(t)
	view, _ := s.View(ctx, raw)
	if _, e := s.Grant(ctx, raw, view.CSRF); e == nil {
		t.Fatal("pending grant")
	}
	other, e := s.Start(ctx, tr)
	if e != nil {
		t.Fatal(e)
	}
	otherView, _ := s.View(ctx, other)
	if e = s.Deny(ctx, raw, otherView.CSRF); e == nil {
		t.Fatal("cross-session CSRF")
	}
	if e = s.Deny(ctx, raw, strings.Repeat("a", 43)); e == nil {
		t.Fatal("wrong CSRF")
	}
	if e = s.Deny(ctx, raw, view.CSRF); e != nil {
		t.Fatal(e)
	}
	if e = s.Deny(ctx, raw, view.CSRF); e == nil {
		t.Fatal("denial replay")
	}
	var codes int
	if e = integrationPool.QueryRow(ctx, `SELECT count(*) FROM oauth_codes WHERE transaction_id=$1`, tr.ID).Scan(&codes); e != nil || codes != 0 {
		t.Fatal("denial issued code")
	}
	if _, e = s.Authenticate(ctx, other, otherView.CSRF); e == nil {
		t.Fatal("consumed transaction authenticated")
	}
	store.now = func() time.Time { return tr.ExpiresAt.Add(time.Second) }
	if _, e = s.View(ctx, other); e == nil {
		t.Fatal("expired session")
	}
}
func TestBrowserRotationRejectsIdentityReassignment(t *testing.T) {
	ctx := context.Background()
	s, store, raw, _ := browserFixture(t)
	view, _ := s.View(ctx, raw)
	next, e := s.Authenticate(ctx, raw, view.CSRF)
	if e != nil {
		t.Fatal(e)
	}
	old, e := store.FindBrowserSession(ctx, oauth.Digest(next))
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"tenant", "user", "issuer", "subject"} {
		n := old
		n.Reference = unique("reference")
		n.Digest = oauth.Digest(unique("credential"))
		switch kind {
		case "tenant":
			n.TenantID = unique("other")
		case "user":
			n.UserID = unique("other")
		case "issuer":
			n.Issuer = "other"
		case "subject":
			n.Subject = "other"
		}
		if e = store.RotateBrowserSession(ctx, old.Digest, old.CSRFHash, n); e == nil {
			t.Fatal("identity reassignment")
		}
	}
}

func TestBrowserRotationExpiryAcrossLockWait(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	service, store, raw, tr := browserFixture(t)
	view, e := service.View(ctx, raw)
	if e != nil {
		t.Fatal(e)
	}
	old, e := store.FindBrowserSession(ctx, oauth.Digest(raw))
	if e != nil {
		t.Fatal(e)
	}
	var clock atomic.Int64
	clock.Store(old.CreatedAt.UnixNano())
	store.now = func() time.Time { return time.Unix(0, clock.Load()).UTC() }
	blocker, e := integrationPool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer blocker.Rollback(context.Background())
	if _, e = blocker.Exec(ctx, `SELECT client_id FROM oauth_clients WHERE client_id=$1 FOR UPDATE`, tr.Request.ClientID); e != nil {
		t.Fatal(e)
	}
	result := make(chan error, 1)
	go func() { _, err := service.Authenticate(ctx, raw, view.CSRF); result <- err }()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if e = integrationPool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::int=ANY(pg_blocking_pids(pid)))`, int32(blocker.Conn().PgConn().PID())).Scan(&waiting); e != nil {
			t.Fatal(e)
		}
		if waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("no browser rotation lock wait")
		case <-ticker.C:
		}
	}
	clock.Store(old.ExpiresAt.Add(time.Second).UnixNano())
	if e = blocker.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-result; e == nil {
		t.Fatal("expired browser session rotated")
	}
	var authenticated int
	var revoked bool
	if e = integrationPool.QueryRow(ctx, `SELECT count(*) FROM browser_sessions WHERE transaction_id=$1 AND state='AUTHENTICATED'`, tr.ID).Scan(&authenticated); e != nil {
		t.Fatal(e)
	}
	if e = integrationPool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM browser_sessions WHERE session_digest=$1`, old.Digest).Scan(&revoked); e != nil {
		t.Fatal(e)
	}
	if authenticated != 0 || revoked {
		t.Fatalf("partial rotation: authenticated=%d revoked=%v", authenticated, revoked)
	}
}
