package requestauth

import (
	"context"
	"errors"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type accessAuditStub struct {
	records []AccessRecord
	fail    string
}

func (s *accessAuditStub) RecordMCPAccess(_ context.Context, r AccessRecord) error {
	s.records = append(s.records, r)
	if r.Decision == s.fail {
		return errors.New("isolated audit fault")
	}
	return nil
}
func TestMCPAuditAdmissionAndFailure(t *testing.T) {
	p, e := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant", ActorID: "user", ClientID: "registered-client", IdentityProvider: "issuer", ProviderSubject: "subject", ExpiresAt: time.Now().Add(time.Hour), Permissions: []auth.Permission{auth.PermissionReadIntent, auth.PermissionReadApproval}})
	if e != nil {
		t.Fatal(e)
	}
	identity, e := auth.NewIdentityWithSubject("user", "issuer", "subject", auth.IdentityStatusActive)
	if e != nil {
		t.Fatal(e)
	}
	resolver := ResolveIdentityFunc(func(context.Context, auth.AuthenticatedPrincipal) (auth.Identity, error) { return identity, nil })
	for _, kind := range []string{"anonymous", "authorized", "RECEIVED", "AUTHORIZED"} {
		t.Run(kind, func(t *testing.T) {
			auditor := &accessAuditStub{fail: kind}
			m, e := NewAuditedOAuthMiddleware(oauthVerifierStub{p}, resolver, auditor)
			if e != nil {
				t.Fatal(e)
			}
			reached := false
			h := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true; w.WriteHeader(204) }))
			r := httptest.NewRequest("POST", "/mcp", nil)
			if kind != "anonymous" {
				r.Header.Set("Authorization", "Bearer fixture-only")
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			want := 503
			if kind == "anonymous" {
				want = 401
			}
			if kind == "authorized" {
				want = 204
			}
			if w.Code != want || reached != (kind == "authorized") {
				t.Fatal("audit bypass", w.Code, reached)
			}
			if len(auditor.records) == 0 || auditor.records[0].Decision != "RECEIVED" || auditor.records[0].TenantID != "" {
				t.Fatal("anonymous attribution")
			}
			if len(auditor.records) == 2 {
				v := auditor.records[1]
				if v.RequestID != auditor.records[0].RequestID || v.TenantID != "tenant" || v.UserID != "user" || v.ClientID != "registered-client" || v.Method != "POST" {
					t.Fatal("audit attribution")
				}
			}
		})
	}
	if _, e = NewAuditedOAuthMiddleware(oauthVerifierStub{p}, resolver, nil); e == nil {
		t.Fatal("missing audit accepted")
	}
}
