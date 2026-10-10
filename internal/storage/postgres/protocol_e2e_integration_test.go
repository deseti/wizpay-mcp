package postgres

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/mcp/tools"
	"github.com/deseti/wizpay-mcp/internal/oauth"
)

type protocolCallResult struct {
	Error *struct {
		Code int `json:"code"`
	} `json:"error"`
	Result struct {
		IsError    bool            `json:"isError"`
		Structured json.RawMessage `json:"structuredContent"`
	} `json:"result"`
}

func TestProtocolSIWEOAuthMCPContinuous(t *testing.T) {
	f := newProtocolFixture(t, "5042")
	login := f.login(t)
	ctx := context.Background()
	// Header attributes and Go's cookie jar establish protocol host/scheme scope,
	// not JavaScript HttpOnly behavior or real browser SameSite enforcement.
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	connect, _ := url.Parse(oauth.Issuer)
	mcp, _ := url.Parse(oauth.Resource)
	plain, _ := url.Parse("http://connect.wizpay.xyz")
	jar.SetCookies(connect, []*http.Cookie{login.authenticated})
	if len(jar.Cookies(connect)) != 1 || len(jar.Cookies(mcp)) != 0 || len(jar.Cookies(plain)) != 0 {
		t.Fatal("session cookie escaped host/HTTPS scope")
	}
	var codes, consents int
	if err = integrationPool.QueryRow(ctx, `SELECT (SELECT count(*) FROM oauth_codes WHERE transaction_id=$1),(SELECT count(*) FROM oauth_consents WHERE client_id=$2)`, login.view.TransactionID, f.client.ID).Scan(&codes, &consents); err != nil || codes != 0 || consents != 0 {
		t.Fatal("SIWE implicitly granted OAuth authority")
	}
	// Stale proof remains invalid after cryptographic authentication and rotation.
	protocolStatus(t, f.browser("POST", "/browser/consent", `{"decision":"grant"}`, login.authenticated, login.pendingView.CSRF), 403)
	code := f.grant(t, login)
	for _, kind := range []string{"verifier", "redirect", "resource"} {
		x := f.exchange(code)
		switch kind {
		case "verifier":
			x.Set("code_verifier", strings.Repeat("x", 43))
		case "redirect":
			x.Set("redirect_uri", "https://attacker.example/callback")
		case "resource":
			x.Set("resource", "https://attacker.example/mcp")
		}
		w := f.browser("POST", "/oauth/token", x.Encode(), nil, "")
		protocolStatus(t, w, 400)
		var failure struct {
			Error string `json:"error"`
		}
		protocolJSON(t, w, &failure)
		expected := "invalid_grant"
		if kind == "resource" {
			expected = "invalid_target"
		}
		if failure.Error != expected {
			t.Fatal("incorrect exchange error for", kind)
		}
		var consumed bool
		if err = integrationPool.QueryRow(ctx, `SELECT consumed_at IS NOT NULL FROM oauth_codes WHERE code_digest=$1`, oauth.Digest(code)).Scan(&consumed); err != nil || consumed {
			t.Fatal("failed exchange consumed code")
		}
	}
	token := f.redeem(t, code)
	protocolStatus(t, f.browser("POST", "/oauth/token", f.exchange(code).Encode(), nil, ""), 400)
	persisted, err := f.store.ValidateOAuthToken(ctx, oauth.Digest(token), f.store.now())
	session, sessionErr := f.store.FindBrowserSession(ctx, oauth.Digest(login.authenticated.Value))
	if err != nil || sessionErr != nil || persisted.Issuer != oauth.Issuer || persisted.Resource != oauth.Resource || persisted.Scope != oauth.ReadScope || persisted.ClientID != f.client.ID || persisted.TenantID != f.base.scope.TenantID() || persisted.UserID != f.base.scope.ActorID() || persisted.SessionID != session.Reference || persisted.ConsentID == "" {
		t.Fatal("persisted token authority binding failed")
	}
	var consumedCodes, issuedTokens, authorizationAudits, tokenAudits int
	err = integrationPool.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM oauth_codes WHERE transaction_id=$1 AND consumed_at IS NOT NULL),
 (SELECT count(*) FROM oauth_access_tokens WHERE consent_id=$2),
 (SELECT count(*) FROM oauth_audit WHERE tenant_id=$3 AND client_id=$4 AND event_type='AUTHORIZED'),
 (SELECT count(*) FROM oauth_audit WHERE tenant_id=$3 AND client_id=$4 AND event_type='TOKEN_ISSUED')`, login.view.TransactionID, persisted.ConsentID, f.base.scope.TenantID(), f.client.ID).Scan(&consumedCodes, &issuedTokens, &authorizationAudits, &tokenAudits)
	if err != nil || consumedCodes != 1 || issuedTokens != 1 || authorizationAudits != 1 || tokenAudits != 1 {
		t.Fatal("code, token and audit lifecycle inconsistent")
	}
	// Real verifier mapping creates no financial/human authority.
	flow, err := oauth.NewService(f.store, f.store.now)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := flow.Verify(ctx, token)
	if err != nil {
		t.Fatal("issued token rejected")
	}
	if !principal.HasPermission(auth.PermissionReadIntent) || !principal.HasPermission(auth.PermissionReadApproval) || len(principal.Permissions()) != 2 {
		t.Fatal("OAuth permission escalation")
	}
	for _, raw := range []string{"", "invalid", "wmcp_at_" + strings.Repeat("z", 43)} {
		w := f.mcp(t, raw, "tools/list", map[string]any{})
		protocolStatus(t, w, 401)
		challenge := `Bearer resource_metadata="` + oauth.MetadataURL + `", scope="` + oauth.ReadScope + `"`
		if raw != "" {
			challenge += `, error="invalid_token"`
		}
		if w.Header().Get("WWW-Authenticate") != challenge || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("incorrect bearer challenge")
		}
	}
	initialize := f.mcp(t, token, "initialize", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "synthetic-protocol-client", "version": "1"}})
	protocolStatus(t, initialize, 200)
	var initialized struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	protocolJSON(t, initialize, &initialized)
	if initialized.Result.ProtocolVersion == "" || initialized.Result.ServerInfo.Name != "wizpay-mcp" {
		t.Fatal("invalid MCP initialization")
	}
	listed := f.mcp(t, token, "tools/list", map[string]any{})
	protocolStatus(t, listed, 200)
	var discovery struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	protocolJSON(t, listed, &discovery)
	names := []string{}
	for _, tool := range discovery.Result.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if !reflect.DeepEqual(names, []string{tools.GetApprovalName, tools.GetIntentName}) {
		t.Fatal("unexpected OAuth tools", names)
	}
	for _, tc := range []struct{ name, field, id string }{{tools.GetIntentName, "intent_id", f.base.intent.IntentID()}, {tools.GetApprovalName, "approval_id", f.base.approval.ApprovalID()}} {
		w := f.mcp(t, token, "tools/call", map[string]any{"name": tc.name, "arguments": map[string]string{"request_id": "protocol-read", tc.field: tc.id}})
		protocolStatus(t, w, 200)
		var result protocolCallResult
		protocolJSON(t, w, &result)
		if result.Error != nil || result.Result.IsError {
			t.Fatal("authorized persisted read rejected")
		}
		var output struct {
			Result map[string]any `json:"result"`
		}
		if json.Unmarshal(result.Result.Structured, &output) != nil || output.Result[tc.field] != tc.id {
			t.Fatal("read returned wrong persisted resource")
		}
	}
	foreign := createBaseFixture(t, false)
	for _, tc := range []struct{ name, field, id string }{{tools.GetIntentName, "intent_id", foreign.intent.IntentID()}, {tools.GetApprovalName, "approval_id", foreign.approval.ApprovalID()}} {
		w := f.mcp(t, token, "tools/call", map[string]any{"name": tc.name, "arguments": map[string]string{"request_id": "foreign-read", tc.field: tc.id}})
		protocolStatus(t, w, 200)
		var result protocolCallResult
		protocolJSON(t, w, &result)
		if result.Error != nil || !result.Result.IsError || strings.Contains(w.Body.String(), tc.id) {
			t.Fatal("cross-tenant resource disclosed")
		}
	}
	excluded := []string{tools.CreateIntentName, tools.RequestApprovalName, tools.EvaluatePolicyName, tools.PrepareExecutionName, tools.SendPreviewName, tools.SendCreateIntentName, tools.SendExecuteName, tools.SendStatusName, tools.PayrollPreviewName, tools.PayrollCreateIntentName, tools.PayrollExecuteName, tools.PayrollStatusName, tools.SwapPreviewName, tools.SwapCreateIntentName, tools.SwapExecuteName, tools.SwapStatusName, tools.ListSchedulesName, tools.GetScheduleName, tools.SimulateScheduleName, tools.CreateScheduleName, tools.ControlScheduleName, tools.EmergencyStopName}
	for _, name := range excluded {
		w := f.mcp(t, token, "tools/call", map[string]any{"name": name, "arguments": map[string]any{}})
		protocolStatus(t, w, 200)
		var denied protocolCallResult
		protocolJSON(t, w, &denied)
		if denied.Error == nil || denied.Error.Code != -32602 {
			t.Fatal("excluded tool callable", name)
		}
	}
	for _, path := range []string{"decision", "authorize-execution"} {
		r := f.request("mcp.wizpay.xyz", "POST", "/approval/"+f.base.approval.ApprovalID()+"/"+path, `{}`, nil, "", token)
		protocolStatus(t, f.send(r), 403)
	}
	// SIWE/browser credentials are not a valid MCP credential class.
	r := f.request("mcp.wizpay.xyz", "POST", "/mcp", `{}`, login.authenticated, "", token)
	protocolStatus(t, f.send(r), 401)
	// Actual logout revokes the linked OAuth session, but does not rewrite the
	// token/consent revocation columns. Live validation rejects the linked token.
	w := f.browser("POST", "/browser/logout", "", login.authenticated, login.view.CSRF)
	protocolStatus(t, w, 200)
	deleted := protocolCookie(t, w)
	if deleted.MaxAge != -1 || deleted.Value != "" {
		t.Fatal("cookie deletion failed")
	}
	protocolStatus(t, f.browser("GET", "/browser/session", "", login.authenticated, ""), 401)
	protocolStatus(t, f.mcp(t, token, "tools/list", map[string]any{}), 401)
	var sessionRevoked, consentRevoked, tokenRevoked bool
	err = integrationPool.QueryRow(ctx, `SELECT s.revoked_at IS NOT NULL,c.revoked_at IS NOT NULL,a.revoked_at IS NOT NULL FROM oauth_access_tokens a JOIN oauth_sessions s ON s.session_id=a.session_id JOIN oauth_consents c ON c.consent_id=a.consent_id WHERE a.token_digest=$1`, oauth.Digest(token)).Scan(&sessionRevoked, &consentRevoked, &tokenRevoked)
	if err != nil || !sessionRevoked || consentRevoked || tokenRevoked {
		t.Fatal("logout lifecycle semantics changed")
	}
}
