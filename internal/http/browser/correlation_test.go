package browser

import (
	"context"
	"encoding/json"
	domain "github.com/deseti/wizpay-mcp/internal/browser"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/deseti/wizpay-mcp/internal/siwe"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type correlationRepo struct {
	*testRepo
	siwe.Repository
	challenge string
}

func (r *correlationRepo) FindBrowserAuthentication(context.Context, domain.Session) (string, error) {
	return r.challenge, nil
}
func TestSessionHTTPServerCorrelation(t *testing.T) {
	now := time.Now()
	repo := &correlationRepo{testRepo: &testRepo{transaction: oauth.Transaction{ID: "transaction-A", CreatedAt: now, ExpiresAt: now.Add(time.Minute), Request: oauth.AuthorizationRequest{ClientID: "client", RedirectURI: "https://client.example/callback", Scope: oauth.ReadScope, Resource: oauth.Resource}}}}
	flow, err := oauth.NewService(repo, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := siwe.NewService(repo, "tenant", siwe.URI)
	if err != nil {
		t.Fatal(err)
	}
	service, err := domain.NewWalletService(repo, repo, flow, verifier, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := service.Start(context.Background(), repo.transaction)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(service)
	get := func() (int, domain.View) {
		r := request("GET", "/browser/session", "")
		r.AddCookie(&http.Cookie{Name: CookieName, Value: raw})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var v domain.View
		_ = json.Unmarshal(w.Body.Bytes(), &v)
		return w.Code, v
	}
	status, v := get()
	if status != 200 || v.TransactionID != "transaction-A" || v.ChallengeID != "" {
		t.Fatal(status, v)
	}
	// Trusted repository fixture supplies evidence; no authentication HTTP shortcut.
	repo.session.State = "AUTHENTICATED"
	repo.session.UserID = "user"
	repo.challenge = strings.Repeat("a", 64)
	status, v = get()
	if status != 200 || v.ChallengeID != repo.challenge {
		t.Fatal(status, v)
	}
	repo.challenge = ""
	status, _ = get()
	if status != 401 {
		t.Fatal("missing evidence", status)
	}
	repo.challenge = strings.Repeat("b", 64)
	status, v = get()
	if status != 200 || v.ChallengeID == strings.Repeat("a", 64) {
		t.Fatal(status, v)
	}
	repo.session.Revoked = true
	status, _ = get()
	if status != 401 {
		t.Fatal("revoked session", status)
	}
}
