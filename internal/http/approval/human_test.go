package approval

import (
	"context"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type humanFixtureAuthenticator struct{ request auth.TrustedRequest }

func (v humanFixtureAuthenticator) AuthenticateHuman(context.Context) (auth.TrustedRequest, error) {
	return v.request, nil
}
func bearerHTTPRequest(t *testing.T, r *http.Request) *http.Request {
	t.Helper()
	p, err := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant", ActorID: "user", IdentityProvider: "issuer", ProviderSubject: "subject", ClientID: "client", ExpiresAt: time.Now().Add(time.Hour), Permissions: []auth.Permission{auth.PermissionDecideApproval, auth.PermissionConfirmExecution}})
	if err != nil {
		t.Fatal(err)
	}
	i, err := auth.NewIdentityWithSubject("user", "issuer", "subject", auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := auth.NewTrustedRequest(p, i, auth.RequestMetadata{RequestID: "request"})
	if err != nil {
		t.Fatal(err)
	}
	return r.WithContext(auth.WithTrustedRequest(r.Context(), trusted))
}
func humanHTTPRequest(t *testing.T, r *http.Request) *http.Request {
	t.Helper()
	r = bearerHTTPRequest(t, r)
	trusted, _ := auth.TrustedRequestFromContext(r.Context())
	ctx, err := auth.AuthenticateHumanContext(r.Context(), humanFixtureAuthenticator{trusted})
	if err != nil {
		t.Fatal(err)
	}
	return r.WithContext(ctx)
}
func TestMCPBearerCannotReachHumanActions(t *testing.T) {
	for _, path := range []string{"decision", "authorize-execution"} {
		s := &serviceStub{}
		h, _ := NewHandler(s)
		w := httptest.NewRecorder()
		r := bearerHTTPRequest(t, httptest.NewRequest(http.MethodPost, "/approval/apr_1/"+path, strings.NewReader(`{"is_human":true,"actor_type":"human","approved_by_user":true}`)))
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || s.decision != "" {
			t.Fatalf("MCP action status=%d decision=%s", w.Code, s.decision)
		}
	}
}
