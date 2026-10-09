package postgres

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/browser"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/deseti/wizpay-mcp/internal/siwe"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/wallet"
	"github.com/ethereum/go-ethereum/crypto"
)

type siweFixture struct {
	service                     *browser.Service
	store                       *Store
	raw, proof, tenant, binding string
	c                           siwe.Challenge
}

func newSIWEFixture(t *testing.T, short bool) siweFixture {
	t.Helper()
	ctx := context.Background()
	f := newOAuthFixture(t)
	store := *integrationStore
	store.now = time.Now
	if _, e := integrationPool.Exec(ctx, `UPDATE tenants SET status='ACTIVE' WHERE tenant_id=$1`, f.f.scope.TenantID()); e != nil {
		t.Fatal(e)
	}
	key, e := crypto.HexToECDSA(strings.Repeat("0", 63) + "1")
	if e != nil {
		t.Fatal(e)
	}
	at := time.Now().UTC()
	id := unique("siwe-binding")
	binding, e := wallet.NewBinding(wallet.BindingParams{BindingID: id, Version: 1, UserID: f.f.scope.ActorID(), Provider: "EXTERNAL_EVM", ProviderUserReference: "test-existing-identity", WalletID: unique("siwe-wallet"), Address: crypto.PubkeyToAddress(key.PublicKey).Hex(), ChainID: "5042", Network: "MAINNET", Status: wallet.BindingStatusActive, VerificationReference: "test-only-reviewed-binding", CreatedAt: at.Add(-time.Minute), VerifiedAt: at.Add(-time.Second)})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.CreateBinding(ctx, f.f.scope, binding); e != nil {
		t.Fatal(e)
	}
	flow, e := oauth.NewService(&store, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	tr, e := flow.Begin(ctx, f.request)
	if e != nil {
		t.Fatal(e)
	}
	verifier, e := siwe.NewService(&store, f.f.scope.TenantID(), siwe.URI)
	if e != nil {
		t.Fatal(e)
	}
	service, e := browser.NewWalletService(&store, &store, flow, verifier, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := service.Start(ctx, tr)
	if e != nil {
		t.Fatal(e)
	}
	if short {
		// Replace only the newly created pending fixture before challenge issuance;
		// production immutable trigger is retained and not disabled. A new short
		// pending session uses the standard repository path and its real deadline.
		v, e := store.FindBrowserSession(ctx, oauth.Digest(raw))
		if e != nil {
			t.Fatal(e)
		}
		raw = "wb_" + oauth.Digest(unique("short-credential"))
		v.Digest = oauth.Digest(raw)
		v.CSRFHash = oauth.Digest(oauth.Digest("wizpay-browser-csrf-v1:" + raw))
		v.Reference = unique("short-reference")
		v.CreatedAt = time.Now().UTC()
		v.ExpiresAt = v.CreatedAt.Add(3 * time.Second)
		if e = store.CreateBrowserSession(ctx, v); e != nil {
			t.Fatal(e)
		}
	}
	view, e := service.View(ctx, raw)
	if e != nil {
		t.Fatal(e)
	}
	c, e := service.Challenge(ctx, raw, view.CSRF, binding.Address())
	if e != nil {
		t.Fatal(e)
	}
	return siweFixture{service, &store, raw, view.CSRF, f.f.scope.TenantID(), id, c}
}
func siweSignature(t *testing.T, message string) string {
	t.Helper()
	key, e := crypto.HexToECDSA(strings.Repeat("0", 63) + "1")
	if e != nil {
		t.Fatal(e)
	}
	hash := crypto.Keccak256([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len([]byte(message)))), []byte(message))
	b, e := crypto.Sign(hash, key)
	if e != nil {
		t.Fatal(e)
	}
	return "0x" + hex.EncodeToString(b)
}
func assertSIWERollback(t *testing.T, f siweFixture) {
	t.Helper()
	var consumed, revoked bool
	var authenticated int
	e := integrationPool.QueryRow(context.Background(), `SELECT consumed_at IS NOT NULL,(SELECT revoked_at IS NOT NULL FROM browser_sessions WHERE session_digest=$2),(SELECT count(*) FROM siwe_authentications WHERE challenge_id=$1) FROM siwe_challenges WHERE challenge_id=$1`, f.c.ID, oauth.Digest(f.raw)).Scan(&consumed, &revoked, &authenticated)
	if e != nil || consumed || revoked || authenticated != 0 {
		t.Fatalf("partial authentication: %v %v %v %d", e, consumed, revoked, authenticated)
	}
}
func TestSIWEConcurrentAuthenticationAndReplay(t *testing.T) {
	f := newSIWEFixture(t, false)
	sig := siweSignature(t, f.c.Message)
	var successes atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	var next string
	var mu sync.Mutex
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			raw, e := f.service.VerifyWallet(context.Background(), f.raw, f.proof, f.c.ID, sig)
			if e == nil {
				successes.Add(1)
				mu.Lock()
				next = raw
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successes %d", successes.Load())
	}
	if _, e := f.service.View(context.Background(), f.raw); e == nil {
		t.Fatal("old credential alive")
	}
	view, e := f.service.View(context.Background(), next)
	if e != nil || view.State != "AUTHENTICATED" {
		t.Fatal(e, view.State)
	}
	if _, e = f.service.Grant(context.Background(), next, view.CSRF); e != nil {
		t.Fatal("separate consent failed", e)
	}
}
func TestSIWERevokedAuthorityAndIsolation(t *testing.T) {
	for _, kind := range []string{"tenant", "identity", "binding", "pending", "missing-evidence", "future-evidence", "session", "transaction", "client", "csrf", "other-session", "wrong-signer"} {
		t.Run(kind, func(t *testing.T) {
			f := newSIWEFixture(t, false)
			ctx := context.Background()
			sig := siweSignature(t, f.c.Message)
			raw, proof := f.raw, f.proof
			var e error
			switch kind {
			case "tenant":
				_, e = integrationPool.Exec(ctx, `UPDATE tenants SET status='REVOKED' WHERE tenant_id=$1`, f.tenant)
			case "identity":
				_, e = integrationPool.Exec(ctx, `UPDATE identities SET status='REVOKED',lifecycle_version=lifecycle_version+1 WHERE tenant_id=$1`, f.tenant)
			case "binding":
				_, e = integrationPool.Exec(ctx, `UPDATE wallet_bindings SET status='REVOKED',revoked_at=clock_timestamp(),version=version+1 WHERE tenant_id=$1 AND binding_id=$2`, f.tenant, f.binding)
			case "pending":
				_, e = integrationPool.Exec(ctx, `UPDATE wallet_bindings SET status='PENDING',verified_at=NULL,verification_reference='',version=version+1 WHERE tenant_id=$1 AND binding_id=$2`, f.tenant, f.binding)
			case "missing-evidence":
				_, e = integrationPool.Exec(ctx, `UPDATE wallet_bindings SET verification_reference='',version=version+1 WHERE tenant_id=$1 AND binding_id=$2`, f.tenant, f.binding)
			case "future-evidence":
				_, e = integrationPool.Exec(ctx, `UPDATE wallet_bindings SET verified_at=clock_timestamp()+interval '1 day',version=version+1 WHERE tenant_id=$1 AND binding_id=$2`, f.tenant, f.binding)
			case "session":
				e = f.service.Logout(ctx, raw, proof)
			case "transaction":
				e = f.service.Deny(ctx, raw, proof)
			case "client":
				_, e = integrationPool.Exec(ctx, `UPDATE oauth_clients SET status='REVOKED' WHERE client_id=$1`, f.c.ClientID)
			case "csrf":
				proof = strings.Repeat("a", 43)
			case "other-session":
				other := newSIWEFixture(t, false)
				raw, proof = other.raw, other.proof
			case "wrong-signer":
				sig = siweSignature(t, f.c.Message+"altered")
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.service.VerifyWallet(ctx, raw, proof, f.c.ID, sig); e == nil {
				t.Fatal("revoked or mismatched authority accepted")
			}
			// Logout intentionally changes the old session; other failures must not.
			if kind != "session" {
				assertSIWERollback(t, f)
			}
		})
	}
}
func TestSIWEExpiryAcrossObservedLockWait(t *testing.T) {
	f := newSIWEFixture(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	blocker, e := integrationPool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer blocker.Rollback(ctx)
	var pid uint32
	if e = blocker.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
		t.Fatal(e)
	}
	if _, e = blocker.Exec(ctx, `SELECT 1 FROM browser_sessions WHERE session_digest=$1 FOR UPDATE`, oauth.Digest(f.raw)); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	sig := siweSignature(t, f.c.Message)
	go func() { _, e := f.service.VerifyWallet(ctx, f.raw, f.proof, f.c.ID, sig); done <- e }()
	waitBrowserBlock(t, ctx, pid)
	// Observe the database wall clock crossing the immutable fixture deadline.
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var expired bool
		if e = integrationPool.QueryRow(ctx, `SELECT clock_timestamp()>=$1::timestamptz`, f.c.ExpiresAt).Scan(&expired); e != nil {
			t.Fatal(e)
		}
		if expired {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	if e = blocker.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e == nil {
		t.Fatal("expired challenge accepted")
	}
	assertSIWERollback(t, f)
}
func TestSIWEAuthenticationAuditFailureRollsBack(t *testing.T) {
	for _, table := range []string{"browser_sessions", "siwe_authentications"} {
		t.Run(table, func(t *testing.T) {
			f := newSIWEFixture(t, false)
			ctx := context.Background()
			fn := unique("siwe_fault")
			trigger := unique("siwe_trigger")
			predicate := "NEW.tenant_id = '" + f.tenant + "'"
			if table == "browser_sessions" {
				predicate += " AND NEW.state='AUTHENTICATED'"
			}
			_, e := integrationPool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF %s THEN RAISE EXCEPTION 'isolated authentication fault'; END IF; RETURN NEW; END $$`, fn, predicate))
			if e != nil {
				t.Fatal(e)
			}
			defer integrationPool.Exec(ctx, "DROP FUNCTION "+fn+"() CASCADE")
			if _, e = integrationPool.Exec(ctx, fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION %s()`, trigger, table, fn)); e != nil {
				t.Fatal(e)
			}
			if _, e = f.service.VerifyWallet(ctx, f.raw, f.proof, f.c.ID, siweSignature(t, f.c.Message)); e == nil {
				t.Fatal("fault did not abort")
			}
			assertSIWERollback(t, f)
		})
	}
}

func TestSIWEAuthenticationCompetesWithLogout(t *testing.T) {
	f := newSIWEFixture(t, false)
	ctx := context.Background()
	sig := siweSignature(t, f.c.Message)
	start := make(chan struct{})
	auth := make(chan error, 1)
	logout := make(chan error, 1)
	go func() { <-start; _, e := f.service.VerifyWallet(ctx, f.raw, f.proof, f.c.ID, sig); auth <- e }()
	go func() { <-start; logout <- f.service.Logout(ctx, f.raw, f.proof) }()
	close(start)
	a, l := <-auth, <-logout
	if (a == nil) == (l == nil) {
		t.Fatalf("exactly one transition must succeed: auth=%v logout=%v", a, l)
	}
}
func TestSIWEChallengeIssuanceBound(t *testing.T) {
	f := newSIWEFixture(t, false)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, e := f.service.Challenge(ctx, f.raw, f.proof, f.c.Address); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := f.service.Challenge(ctx, f.raw, f.proof, f.c.Address); e == nil {
		t.Fatal("challenge limit exceeded")
	}
}
func TestSIWETenantRemainsInactiveByDefault(t *testing.T) {
	f := createBaseFixture(t, false)
	var status string
	if e := integrationPool.QueryRow(context.Background(), `SELECT status FROM tenants WHERE tenant_id=$1`, f.scope.TenantID()).Scan(&status); e != nil || status != "INACTIVE" {
		t.Fatal(e, status)
	}
}

func TestSIWEChallengeExpiryAfterTenantLock(t *testing.T) {
	f := newSIWEFixture(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	// Insert a separate short-lived issued fixture; never mutate the immutable
	// production challenge or disable its trigger.
	var e error
	f.c.ID, e = siwe.Random()
	if e != nil {
		t.Fatal(e)
	}
	f.c.Nonce, e = siwe.Random()
	if e != nil {
		t.Fatal(e)
	}
	f.c.IssuedAt = time.Now().UTC()
	f.c.ExpiresAt = f.c.IssuedAt.Add(3 * time.Second)
	f.c.Message, e = siwe.Message(f.c)
	if e != nil {
		t.Fatal(e)
	}
	_, e = integrationPool.Exec(ctx, `INSERT INTO siwe_challenges(challenge_id,nonce,message,address,tenant_id,session_reference,transaction_id,client_id,domain,uri,chain_id,issued_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'connect.wizpay.xyz','https://connect.wizpay.xyz','5042',$9,$10)`, f.c.ID, f.c.Nonce, f.c.Message, f.c.Address, f.c.TenantID, f.c.SessionReference, f.c.TransactionID, f.c.ClientID, f.c.IssuedAt, f.c.ExpiresAt)
	if e != nil {
		t.Fatal(e)
	}
	blocker, e := integrationPool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer blocker.Rollback(ctx)
	var pid uint32
	if e = blocker.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
		t.Fatal(e)
	}
	if _, e = blocker.Exec(ctx, `SELECT tenant_id FROM tenants WHERE tenant_id=$1 FOR UPDATE`, f.tenant); e != nil {
		t.Fatal(e)
	}
	sig := siweSignature(t, f.c.Message)
	done := make(chan error, 1)
	go func() { _, e := f.service.VerifyWallet(ctx, f.raw, f.proof, f.c.ID, sig); done <- e }()
	waitBrowserBlock(t, ctx, pid)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var expired bool
		if e = integrationPool.QueryRow(ctx, `SELECT clock_timestamp()>=$1::timestamptz`, f.c.ExpiresAt).Scan(&expired); e != nil {
			t.Fatal(e)
		}
		if expired {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	if e = blocker.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e == nil {
		t.Fatal("expiry bypass after tenant lock")
	}
	assertSIWERollback(t, f)
}
func TestSIWEAmbiguousWalletRejected(t *testing.T) {
	f := newSIWEFixture(t, false)
	ctx := context.Background()
	_, e := integrationPool.Exec(ctx, `INSERT INTO wallet_bindings(tenant_id,binding_id,version,user_id,provider,provider_user_reference,wallet_id,address,chain_id,network,status,verification_reference,created_at,verified_at) SELECT tenant_id,$3,1,user_id,provider,provider_user_reference,$4,lower(address),chain_id,network,status,verification_reference,created_at,verified_at FROM wallet_bindings WHERE tenant_id=$1 AND binding_id=$2`, f.tenant, f.binding, unique("ambiguous-binding"), unique("ambiguous-wallet"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.VerifyWallet(ctx, f.raw, f.proof, f.c.ID, siweSignature(t, f.c.Message)); e == nil {
		t.Fatal("ambiguous wallet accepted")
	}
	assertSIWERollback(t, f)
}

func TestSIWESessionCorrelation(t *testing.T) {
	ctx := context.Background()
	f := newSIWEFixture(t, false)
	pending, err := f.service.View(ctx, f.raw)
	if err != nil || pending.TransactionID != f.c.TransactionID || pending.ChallengeID != "" {
		t.Fatalf("pending correlation: %+v %v", pending, err)
	}
	next, err := f.service.VerifyWallet(ctx, f.raw, f.proof, f.c.ID, siweSignature(t, f.c.Message))
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.service.View(ctx, next)
	if err != nil || first.ChallengeID != f.c.ID || first.TransactionID != pending.TransactionID {
		t.Fatalf("authenticated correlation: %+v %v", first, err)
	}
	if _, err = f.service.View(ctx, f.raw); err == nil {
		t.Fatal("rotated credential accepted")
	}
	// A second transaction for the SAME client and existing wallet must retain
	// distinct evidence. Selecting its cookie cannot masquerade as challenge A.
	original, err := f.store.FindOAuthTransaction(ctx, f.c.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := oauth.NewService(f.store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := flow.Begin(ctx, original.Request)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := f.service.Start(ctx, tr)
	if err != nil {
		t.Fatal(err)
	}
	v, err := f.service.View(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	// A distinct reviewed wallet for the same owner exercises same-client wallet
	// replacement without provisioning a new identity or changing tenant policy.
	oldSession, err := f.store.FindBrowserSession(ctx, oauth.Digest(next))
	if err != nil {
		t.Fatal(err)
	}
	scope, err := storage.NewScope(f.tenant, oldSession.UserID, "test-correlation", "test-correlation")
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := crypto.HexToECDSA(strings.Repeat("0", 63) + "2")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	binding, err := wallet.NewBinding(wallet.BindingParams{BindingID: unique("correlation-binding"), Version: 1, UserID: oldSession.UserID, Provider: "EXTERNAL_EVM", ProviderUserReference: "test-existing-identity", WalletID: unique("correlation-wallet"), Address: crypto.PubkeyToAddress(secondKey.PublicKey).Hex(), ChainID: "5042", Network: "MAINNET", Status: wallet.BindingStatusActive, VerificationReference: "test-reviewed-binding", CreatedAt: at.Add(-time.Minute), VerifiedAt: at.Add(-time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.CreateBinding(ctx, scope, binding); err != nil {
		t.Fatal(err)
	}
	c, err := f.service.Challenge(ctx, raw, v.CSRF, binding.Address())
	if err != nil {
		t.Fatal(err)
	}
	hash := crypto.Keccak256([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len([]byte(c.Message)))), []byte(c.Message))
	sig, err := crypto.Sign(hash, secondKey)
	if err != nil {
		t.Fatal(err)
	}
	secondRaw, err := f.service.VerifyWallet(ctx, raw, v.CSRF, c.ID, "0x"+hex.EncodeToString(sig))
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.View(ctx, secondRaw)
	if err != nil || second.ClientID != first.ClientID || second.ChallengeID == first.ChallengeID || second.TransactionID == first.TransactionID {
		t.Fatalf("substituted correlation: %+v %v", second, err)
	}
	if err = f.service.Logout(ctx, secondRaw, second.CSRF); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.View(ctx, secondRaw); err == nil {
		t.Fatal("revoked correlation exposed")
	}
	if _, err = f.service.View(ctx, next); err != nil {
		t.Fatal("other session was revoked", err)
	}
}

func TestSIWECorrelationRequiresPersistentEvidence(t *testing.T) {
	ctx := context.Background()
	f := newSIWEFixture(t, false)
	pending, err := f.store.FindBrowserSession(ctx, oauth.Digest(f.raw))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.FindBrowserAuthentication(ctx, pending); err == nil {
		t.Fatal("pending session accepted as evidence")
	}
	// Direct internal fixture row has no immutable authentication proof. It must
	// never yield correlation, even though its state/owner metadata look valid.
	missing := browser.Session{Digest: oauth.Digest("test-only-missing-evidence"), CSRFHash: oauth.Digest("test-only-csrf"), Reference: unique("missing-proof"), TransactionID: pending.TransactionID, State: "AUTHENTICATED", Decision: "OPEN", TenantID: f.tenant, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}
	if err = integrationPool.QueryRow(ctx, `SELECT user_id,provider,provider_subject FROM identities WHERE tenant_id=$1`, f.tenant).Scan(&missing.UserID, &missing.Issuer, &missing.Subject); err != nil {
		t.Fatal(err)
	}
	if _, err = integrationPool.Exec(ctx, `INSERT INTO browser_sessions(session_digest,csrf_digest,session_reference,transaction_id,state,decision,tenant_id,user_id,identity_issuer,subject,evidence_reference,created_at,expires_at) VALUES($1,$2,$3,$4,'AUTHENTICATED','OPEN',$5,$6,$7,$8,'test-missing-proof',$9,$10)`, missing.Digest, missing.CSRFHash, missing.Reference, missing.TransactionID, missing.TenantID, missing.UserID, missing.Issuer, missing.Subject, missing.CreatedAt, missing.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.FindBrowserAuthentication(ctx, missing); err == nil {
		t.Fatal("missing immutable evidence accepted")
	}
}
