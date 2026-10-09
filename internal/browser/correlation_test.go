package browser

import (
	"context"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/deseti/wizpay-mcp/internal/siwe"
	"strings"
	"testing"
	"time"
)

type correlatedFixture struct {
	*fixtureRepo
	challenge string
	failure   error
}

func (r *correlatedFixture) FindBrowserAuthentication(_ context.Context, _ Session) (string, error) {
	return r.challenge, r.failure
}
func TestVerifiedSessionCorrelation(t *testing.T) {
	s, r, raw := fixture(t)
	repo := &correlatedFixture{fixtureRepo: r, challenge: strings.Repeat("a", 64)}
	s.repo = repo
	// Trusted fixture only; production constructs its verifier through existing wiring.
	s.wallet = &siwe.Service{}
	v := r.records[oauth.Digest(raw)]
	v.State = "AUTHENTICATED"
	v.UserID = "user"
	r.records[v.Digest] = v
	result, err := s.View(context.Background(), raw)
	if err != nil || result.ChallengeID != repo.challenge || result.TransactionID != v.TransactionID {
		t.Fatalf("correlation %+v %v", result, err)
	}
	repo.challenge = ""
	if _, err = s.View(context.Background(), raw); err == nil {
		t.Fatal("missing evidence accepted")
	}
	repo.challenge = strings.Repeat("b", 64)
	result, err = s.View(context.Background(), raw)
	if err != nil || result.ChallengeID == strings.Repeat("a", 64) {
		t.Fatal("evidence substituted")
	}
	v.Revoked = true
	r.records[v.Digest] = v
	if _, err = s.View(context.Background(), raw); err == nil {
		t.Fatal("revoked session accepted")
	}
	v.Revoked = false
	v.ExpiresAt = time.Now().Add(-time.Second)
	r.records[v.Digest] = v
	if _, err = s.View(context.Background(), raw); err == nil {
		t.Fatal("expired session accepted")
	}
}
