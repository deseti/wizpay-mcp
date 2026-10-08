package app

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/config"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
	"github.com/deseti/wizpay-mcp/internal/services"
)

type metadataOnlyRepository struct{ oauth.Repository }

func TestOAuthServerRouteBoundaries(t *testing.T) {
	s, e := oauth.NewService(metadataOnlyRepository{}, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	resolver := requestauth.ResolveIdentityFunc(func(_ context.Context, _ auth.AuthenticatedPrincipal) (auth.Identity, error) {
		t.Fatal("missing token resolved identity")
		return auth.Identity{}, nil
	})
	m, e := requestauth.NewOAuthMiddleware(s, resolver)
	if e != nil {
		t.Fatal(e)
	}
	cfg := config.Config{AppEnv: "test", ServerPort: 8080, LogLevel: "info", OAuthEnabled: true, Auth: config.AuthConfig{Required: true}}
	server, e := NewOAuthServerWithApproval(cfg, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), nil, m.Wrap, &services.PersistedApprovalService{}, s)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		host, path string
		status     int
	}{{"mcp.wizpay.xyz", "/.well-known/oauth-protected-resource/mcp", 200}, {"connect.wizpay.xyz", "/.well-known/oauth-authorization-server", 200}, {"mcp.wizpay.xyz", "/mcp", 401}, {"connect.wizpay.xyz", "/mcp", 400}, {"connect.wizpay.xyz", "/approval/id/decision", 400}, {"mcp.wizpay.xyz", "/oauth/token", 404}, {"attacker.example", "/.well-known/oauth-protected-resource", 404}} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.Host = tc.host
		w := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s status=%d", tc.host, tc.path, w.Code)
		}
		if tc.status == 401 && !strings.Contains(w.Header().Get("WWW-Authenticate"), oauth.MetadataURL) {
			t.Fatal("discovery challenge missing")
		}
	}
}
