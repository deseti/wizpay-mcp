package postgres

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/oauth"
)

func TestProtocolConcurrentCodeRedemption(t *testing.T) {
	f := newProtocolFixture(t, "5042")
	login := f.login(t)
	code := f.grant(t, login)
	type outcome struct {
		status    int
		token     oauth.TokenResponse
		failure   string
		malformed bool
	}
	results := make(chan outcome, 8)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			w := f.browser("POST", "/oauth/token", f.exchange(code).Encode(), nil, "")
			var token oauth.TokenResponse
			var failure struct {
				Error string `json:"error"`
			}
			malformed := json.Unmarshal(w.Body.Bytes(), &token) != nil || json.Unmarshal(w.Body.Bytes(), &failure) != nil
			results <- outcome{w.Code, token, failure.Error, malformed}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	valid := ""
	for r := range results {
		if r.malformed {
			t.Fatal("malformed concurrent protocol response")
		}
		switch r.status {
		case 200:
			if r.token.TokenType != "Bearer" || r.token.Scope != oauth.ReadScope || r.token.AccessToken == "" {
				t.Fatal("invalid concurrent token profile")
			}
			successes++
			valid = r.token.AccessToken
		case 400:
			if r.failure != "invalid_grant" {
				t.Fatal("incorrect concurrent redemption rejection")
			}
		default:
			t.Fatalf("concurrent redemption status=%d", r.status)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent redemption successes=%d, want=1", successes)
	}
	var tokens, audits int
	err := integrationPool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM oauth_access_tokens WHERE client_id=$1),(SELECT count(*) FROM oauth_audit WHERE client_id=$1 AND event_type='TOKEN_ISSUED')`, f.client.ID).Scan(&tokens, &audits)
	if err != nil || tokens != 1 || audits != 1 {
		t.Fatal("concurrent token issuance is not atomic")
	}
	protocolStatus(t, f.mcp(t, valid, "tools/list", map[string]any{}), 200)
}
