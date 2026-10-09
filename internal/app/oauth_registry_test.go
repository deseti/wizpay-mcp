package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/config"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/mcp/tools"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
	"github.com/deseti/wizpay-mcp/internal/services"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

// Fixtures implement only token validation and reads. Any unexpected mutation
// reaches an unimplemented embedded port and fails the test rather than doing work.
type readOAuthRepository struct {
	oauth.Repository
	tokens map[string]oauth.Token
}

type readPrincipalVerifier struct{ principal auth.AuthenticatedPrincipal }

func (v readPrincipalVerifier) Verify(context.Context, string) (auth.AuthenticatedPrincipal, error) {
	return v.principal, nil
}

func (r readOAuthRepository) ValidateOAuthToken(_ context.Context, digest string, _ time.Time) (oauth.Token, error) {
	token, ok := r.tokens[digest]
	if !ok {
		return oauth.Token{}, oauth.ErrDenied
	}
	return token, nil
}

type scopedReadRepository struct {
	storage.IntentRepository
	storage.ApprovalRepository
	reads int
}

func (r *scopedReadRepository) check(scope storage.Scope) error {
	r.reads++
	if scope.TenantID() != "tenant" || scope.ActorID() != "user" {
		return oauth.ErrDenied
	}
	return nil
}
func (r *scopedReadRepository) FindIntentByID(_ context.Context, scope storage.Scope, _ string) (intents.Intent, error) {
	return intents.Intent{}, r.check(scope)
}
func (r *scopedReadRepository) FindApprovalByID(_ context.Context, scope storage.Scope, _ string) (approvals.Approval, error) {
	return approvals.Approval{}, r.check(scope)
}

func TestOAuthReadOnlyMCPTransport(t *testing.T) {
	now := time.Now()
	valid := "wmcp_at_" + strings.Repeat("a", 43)
	other := "wmcp_at_" + strings.Repeat("b", 43)
	unsupported := "wmcp_at_" + strings.Repeat("c", 43)
	base := oauth.Token{Issuer: oauth.Issuer, Resource: oauth.Resource, ClientID: "client", TenantID: "tenant", UserID: "user", IdentityIssuer: "issuer", Subject: "subject", Scope: oauth.ReadScope, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	foreign := base
	foreign.TenantID = "other-tenant"
	escalated := base
	escalated.Scope = "approval:decide:human"
	verifier, err := oauth.NewService(readOAuthRepository{tokens: map[string]oauth.Token{oauth.Digest(valid): base, oauth.Digest(other): foreign, oauth.Digest(unsupported): escalated}}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	identity, err := auth.NewIdentityWithSubject("user", "issuer", "subject", auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	middleware, err := requestauth.NewOAuthMiddleware(verifier, requestauth.ResolveIdentityFunc(func(context.Context, auth.AuthenticatedPrincipal) (auth.Identity, error) { return identity, nil }))
	if err != nil {
		t.Fatal(err)
	}
	repo := &scopedReadRepository{}
	authorizer := auth.NewPermissionAuthorizer()
	intentService := &services.PersistedIntentService{Intents: repo, Authorizer: authorizer}
	approvalService := &services.PersistedApprovalService{Approvals: repo, Authorizer: authorizer}
	registry, err := tools.NewOAuthReadOnlyRegistry(intentService, approvalService)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AppEnv: "test", ServerPort: 8080, LogLevel: "info", OAuthEnabled: true, Auth: config.AuthConfig{Required: true}}
	server, err := NewOAuthServerWithApproval(cfg, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), nil, middleware.Wrap, approvalService, verifier, registry.Tools()...)
	if err != nil {
		t.Fatal(err)
	}
	request := func(token, method string, params any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		r.Host = "mcp.wizpay.xyz"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(w, r)
		return w
	}
	for _, token := range []string{"", "invalid", unsupported} {
		w := request(token, "tools/list", map[string]any{})
		if w.Code != 401 && w.Code != 403 {
			t.Fatalf("invalid authority accepted: %d %s", w.Code, w.Body.String())
		}
	}
	w := request(valid, "initialize", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "read-only-test", "version": "1"}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"protocolVersion"`) {
		t.Fatalf("initialize: %d %s", w.Code, w.Body.String())
	}
	w = request(valid, "tools/list", map[string]any{})
	var listed struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &listed) != nil {
		t.Fatalf("discovery: %d %s", w.Code, w.Body.String())
	}
	names := []string{}
	for _, tool := range listed.Result.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if !reflect.DeepEqual(names, []string{tools.GetApprovalName, tools.GetIntentName}) {
		t.Fatalf("discovered tools: %v", names)
	}
	excluded := []string{tools.CreateIntentName, tools.RequestApprovalName, tools.EvaluatePolicyName, tools.PrepareExecutionName, tools.SendPreviewName, tools.SendCreateIntentName, tools.SendExecuteName, tools.SendStatusName, tools.PayrollPreviewName, tools.PayrollCreateIntentName, tools.PayrollExecuteName, tools.PayrollStatusName, tools.SwapPreviewName, tools.SwapCreateIntentName, tools.SwapExecuteName, tools.SwapStatusName, tools.ListSchedulesName, tools.GetScheduleName, tools.SimulateScheduleName, tools.CreateScheduleName, tools.ControlScheduleName, tools.EmergencyStopName}
	for _, name := range excluded {
		w := request(valid, "tools/call", map[string]any{"name": name, "arguments": map[string]any{}})
		var denied struct {
			Error *struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &denied) != nil || denied.Error == nil || denied.Error.Code != -32602 {
			t.Fatalf("excluded %s: %d %s", name, w.Code, w.Body.String())
		}
	}
	if repo.reads != 0 {
		t.Fatal("excluded calls invoked read services")
	}
	for _, tc := range []struct{ name, field string }{{tools.GetIntentName, "intent_id"}, {tools.GetApprovalName, "approval_id"}} {
		for _, token := range []string{valid, other} {
			w := request(token, "tools/call", map[string]any{"name": tc.name, "arguments": map[string]string{"request_id": "read-test", tc.field: "resource"}})
			var result struct {
				Result struct {
					IsError    bool           `json:"isError"`
					Structured map[string]any `json:"structuredContent"`
				} `json:"result"`
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Result.Structured == nil || result.Result.IsError != (token == other) {
				t.Fatalf("read %s: %d %s", tc.name, w.Code, w.Body.String())
			}
		}
	}
	if repo.reads != 4 {
		t.Fatalf("read count %d", repo.reads)
	}
	p, err := verifier.Verify(context.Background(), valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range []auth.Permission{auth.PermissionDecideApproval, auth.PermissionConfirmExecution, auth.PermissionCreateIntent, auth.PermissionPrepareExecution, auth.PermissionAutonomyControl} {
		if p.HasPermission(permission) {
			t.Fatal(fmt.Sprintf("read token grants %s", permission))
		}
	}
	// Exercise the middleware's insufficient_scope branch through the same
	// application transport, even if a verifier returns a partial read profile.
	partial, err := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant", ActorID: "user", IdentityProvider: "issuer", ProviderSubject: "subject", ExpiresAt: now.Add(time.Hour), Permissions: []auth.Permission{auth.PermissionReadIntent}})
	if err != nil {
		t.Fatal(err)
	}
	limited, err := requestauth.NewOAuthMiddleware(readPrincipalVerifier{partial}, requestauth.ResolveIdentityFunc(func(context.Context, auth.AuthenticatedPrincipal) (auth.Identity, error) {
		t.Fatal("insufficient scope resolved identity")
		return auth.Identity{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	server, err = NewOAuthServerWithApproval(cfg, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), nil, limited.Wrap, approvalService, verifier, registry.Tools()...)
	if err != nil {
		t.Fatal(err)
	}
	w = request(valid, "tools/list", map[string]any{})
	if w.Code != 403 || !strings.Contains(w.Header().Get("WWW-Authenticate"), "insufficient_scope") || repo.reads != 4 {
		t.Fatal("insufficient scope reached MCP transport")
	}
}
