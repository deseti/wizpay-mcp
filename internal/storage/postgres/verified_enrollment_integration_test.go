package postgres

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/browser"
	"github.com/deseti/wizpay-mcp/internal/mcp/tools"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/deseti/wizpay-mcp/internal/siwe"
	"github.com/jackc/pgx/v5"
)

func TestVerifiedEnrollmentOwnerDecisionAndProductionProtocol(t *testing.T) {
	ctx := context.Background()
	f := newProtocolFixture(t, "5042", true)
	cookie, view := f.start(t)
	w := f.browser("POST", "/browser/siwe/challenge", protocolBody(t, map[string]string{"address": f.address}), cookie, view.CSRF)
	protocolStatus(t, w, 200)
	var c struct {
		ID      string `json:"challenge_id"`
		Message string `json:"message"`
		Purpose string `json:"purpose"`
	}
	protocolJSON(t, w, &c)
	if c.Purpose != "ENROLL" || !strings.Contains(c.Message, "Owner approval is required") {
		t.Fatal("wrong enrollment purpose")
	}
	verify := func(signature string) *httptest.ResponseRecorder {
		return f.browser("POST", "/browser/siwe/verify", protocolBody(t, map[string]string{"challenge_id": c.ID, "signature": signature}), cookie, view.CSRF)
	}
	// Cross-message signatures do not establish proof.
	protocolStatus(t, verify(siweSignature(t, c.Message+" tampered")), 403)
	protocolStatus(t, verify(siweSignature(t, c.Message)), 200)
	pending := f.view(t, cookie)
	if pending.State != "PENDING" {
		t.Fatal("proof authenticated without owner")
	}
	protocolStatus(t, f.browser("POST", "/browser/consent", `{"decision":"grant"}`, cookie, view.CSRF), 403)
	protocolStatus(t, verify(siweSignature(t, c.Message)), 403)
	var binding, user string
	if err := integrationPool.QueryRow(ctx, `SELECT binding_id,user_id FROM wallet_enrollment_proofs WHERE challenge_id=$1`, c.ID).Scan(&binding, &user); err != nil {
		t.Fatal(err)
	}
	admin := onboardingLogin(t, f.base.scope.TenantID())
	var login string
	if err := admin.QueryRow(ctx, "SELECT session_user").Scan(&login); err != nil {
		t.Fatal(err)
	}
	if _, err := integrationPool.Exec(ctx, "GRANT EXECUTE ON FUNCTION admin_activate_wallet(text,text,text,text,text,text,timestamptz) TO "+pgx.Identifier{login}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	activate := func(request, tenant, actor, proof string) error {
		_, err := admin.Exec(ctx, `SELECT admin_activate_wallet($1,$2,$3,$4,$5,'test-owner-approval',clock_timestamp()+interval '1 minute')`, request, tenant, actor, binding, proof)
		return err
	}
	for _, bad := range []struct{ tenant, user, proof string }{{f.base.scope.TenantID(), user, strings.Repeat("a", 64)}, {f.base.scope.TenantID(), unique("other-user"), c.ID}, {unique("other-tenant"), user, c.ID}} {
		if activate(unique("denied"), bad.tenant, bad.user, bad.proof) == nil {
			t.Fatal("invalid approval accepted")
		}
	}
	for _, sql := range []string{`INSERT INTO wallet_enrollment_proofs SELECT * FROM wallet_enrollment_proofs`, `UPDATE wallet_enrollment_approvals SET review_reference='forged'`, `UPDATE wallet_bindings SET status='ACTIVE'`} {
		if _, err := admin.Exec(ctx, sql); err == nil {
			t.Fatal("administrator manufactured authority")
		}
	}
	// Two distinct authorized requests must not both activate the same proof/binding.
	second := onboardingLogin(t, f.base.scope.TenantID())
	var secondRole string
	if err := second.QueryRow(ctx, "SELECT session_user").Scan(&secondRole); err != nil {
		t.Fatal(err)
	}
	if _, err := integrationPool.Exec(ctx, "GRANT EXECUTE ON FUNCTION admin_activate_wallet(text,text,text,text,text,text,timestamptz) TO "+pgx.Identifier{secondRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for _, conn := range []*pgx.Conn{admin, second} {
		wg.Add(1)
		go func(conn *pgx.Conn) {
			defer wg.Done()
			_, err := conn.Exec(ctx, `SELECT admin_activate_wallet($1,$2,$3,$4,$5,'test-owner-approval',clock_timestamp()+interval '1 minute')`, unique("activate"), f.base.scope.TenantID(), user, binding, c.ID)
			if err == nil {
				successes.Add(1)
			}
		}(conn)
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("concurrent activation", successes.Load())
	}
	if activate(unique("replay"), f.base.scope.TenantID(), user, c.ID) == nil {
		t.Fatal("activation replay")
	}
	// Enrollment signature cannot authenticate; a fresh challenge is required.
	loginFlow := f.login(t)
	code := f.grant(t, loginFlow)
	token := f.redeem(t, code)
	protocolStatus(t, f.mcp(t, token, "tools/list", map[string]any{}), 200)
	for _, call := range []struct{ name, field, id string }{{tools.GetIntentName, "intent_id", f.base.intent.IntentID()}, {tools.GetApprovalName, "approval_id", f.base.approval.ApprovalID()}} {
		result := f.mcp(t, token, "tools/call", map[string]any{"name": call.name, "arguments": map[string]string{"request_id": "owner-read", call.field: call.id}})
		protocolStatus(t, result, 200)
		var read protocolCallResult
		protocolJSON(t, result, &read)
		if read.Error != nil || read.Result.IsError {
			t.Fatal("runtime authorized read denied")
		}
	}
	var authorized int
	if err := integrationPool.QueryRow(ctx, `SELECT count(*) FROM mcp_access_audit WHERE tenant_id=$1 AND user_id=$2 AND client_id=$3 AND decision='AUTHORIZED'`, f.base.scope.TenantID(), user, f.client.ID).Scan(&authorized); err != nil || authorized != 3 {
		t.Fatal("MCP audit attribution", err, authorized)
	}
	if _, err := admin.Exec(ctx, `SELECT admin_onboarding('REVOKE_BINDING',$1,$2,$3,$4,'','','','test-revoke',clock_timestamp()+interval '1 minute')`, unique("revoke"), f.base.scope.TenantID(), user, binding); err != nil {
		t.Fatal(err)
	}
	protocolStatus(t, f.browser("GET", "/browser/session", "", loginFlow.authenticated, ""), 401)
	protocolStatus(t, f.mcp(t, token, "tools/list", map[string]any{}), 401)
	if activate(unique("restore"), f.base.scope.TenantID(), user, c.ID) == nil {
		t.Fatal("revoked authority restored")
	}
}

func TestProductionSIWERejectsUnverifiedMetadata(t *testing.T) {
	f := newSIWEFixture(t, false)
	verifier, err := siwe.NewVerifiedService(f.store, f.tenant, siwe.URI)
	if err != nil {
		t.Fatal(err)
	}
	service, err := browser.NewWalletService(f.store, f.store, mustOAuth(t, f.store), verifier, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.VerifyWallet(context.Background(), f.raw, f.proof, f.c.ID, siweSignature(t, f.c.Message)); err == nil {
		t.Fatal("arbitrary metadata authenticated")
	}
	assertSIWERollback(t, f)
}
func mustOAuth(t *testing.T, s *Store) *oauth.Service {
	t.Helper()
	v, e := oauth.NewService(s, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
