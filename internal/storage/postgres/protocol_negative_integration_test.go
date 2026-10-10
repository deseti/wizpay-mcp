package postgres

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/deseti/wizpay-mcp/internal/siwe"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestProtocolPendingAuthorityRejections(t *testing.T) {
	f := newProtocolFixture(t, "5042")
	cookie, view := f.start(t)
	other, otherView := f.start(t)
	for _, tc := range []struct {
		name, proof, origin string
		status              int
	}{
		{"missing-csrf", "", oauth.Issuer, 403},
		{"invalid-csrf", strings.Repeat("x", 43), oauth.Issuer, 403},
		{"cross-session-csrf", otherView.CSRF, oauth.Issuer, 403},
		{"wrong-origin", view.CSRF, "https://attacker.example", 403},
		{"pending-grant", view.CSRF, oauth.Issuer, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := f.request("connect.wizpay.xyz", "POST", "/browser/consent", `{"decision":"grant"}`, cookie, tc.proof, "")
			r.Header.Set("Origin", tc.origin)
			protocolStatus(t, f.send(r), tc.status)
		})
	}
	// Browser identity claims and arbitrary chain selection are not supported inputs.
	protocolStatus(t, f.browser("POST", "/browser/siwe/challenge", protocolBody(t, map[string]string{"address": f.address, "chain_id": "1"}), cookie, view.CSRF), 400)
	protocolStatus(t, f.browser("POST", "/browser/siwe/challenge", protocolBody(t, map[string]string{"address": f.address, "user_id": "other"}), cookie, view.CSRF), 400)
	var codes int
	if err := integrationPool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_codes WHERE client_id=$1`, f.client.ID).Scan(&codes); err != nil || codes != 0 {
		t.Fatal("pending request created code authority")
	}
	w := f.browser("POST", "/browser/siwe/challenge", protocolBody(t, map[string]string{"address": f.address}), cookie, view.CSRF)
	protocolStatus(t, w, 200)
	var c struct {
		ID      string `json:"challenge_id"`
		Message string `json:"message"`
	}
	protocolJSON(t, w, &c)
	// A valid proof for a different pending cookie does not transfer the challenge.
	protocolStatus(t, f.browser("POST", "/browser/siwe/verify", protocolBody(t, map[string]string{"challenge_id": c.ID, "signature": siweSignature(t, c.Message)}), other, otherView.CSRF), 403)
	protocolStatus(t, f.browser("POST", "/browser/consent", `{"decision":"deny"}`, cookie, view.CSRF), 200)
	if f.view(t, cookie).State != "DENIED" {
		t.Fatal("denial state missing")
	}
	protocolStatus(t, f.browser("POST", "/browser/siwe/verify", protocolBody(t, map[string]string{"challenge_id": c.ID, "signature": siweSignature(t, c.Message)}), cookie, view.CSRF), 403)
	if err := integrationPool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_codes WHERE client_id=$1`, f.client.ID).Scan(&codes); err != nil || codes != 0 {
		t.Fatal("denial issued authorization code")
	}
}

func TestProtocolAuthorizationRequestBindings(t *testing.T) {
	f := newProtocolFixture(t, "5042")
	for _, field := range []string{"redirect_uri", "resource", "code_challenge_method", "scope"} {
		q := f.authorizeQuery()
		switch field {
		case "redirect_uri":
			q.Set(field, "https://attacker.example/callback")
		case "resource":
			q.Set(field, "https://attacker.example/mcp")
		case "code_challenge_method":
			q.Set(field, "plain")
		case "scope":
			q.Set(field, "approval:decide:human")
		}
		w := f.browser("GET", "/oauth/authorize?"+q.Encode(), "", nil, "")
		protocolStatus(t, w, 400)
		if len(w.Result().Cookies()) != 0 || w.Header().Get("Location") != "" {
			t.Fatal("invalid authorization established browser authority")
		}
	}
}

func TestProtocolSIWEReplayAndExpiredAuthority(t *testing.T) {
	t.Run("consumed-challenge", func(t *testing.T) {
		f := newProtocolFixture(t, "5042")
		l := f.login(t)
		body := protocolBody(t, map[string]string{"challenge_id": l.challenge.ID, "signature": siweSignature(t, l.challenge.Message)})
		protocolStatus(t, f.browser("POST", "/browser/siwe/verify", body, l.pending, l.pendingView.CSRF), 403)
		c, v := f.start(t)
		protocolStatus(t, f.browser("POST", "/browser/siwe/verify", body, c, v.CSRF), 403)
		var count int
		if err := integrationPool.QueryRow(context.Background(), `SELECT count(*) FROM siwe_authentications WHERE challenge_id=$1`, l.challenge.ID).Scan(&count); err != nil || count != 1 {
			t.Fatal("challenge replay produced authority")
		}
	})
	t.Run("expired-challenge", func(t *testing.T) {
		f := newProtocolFixture(t, "5042")
		cookie, view := f.start(t)
		session, err := f.store.FindBrowserSession(context.Background(), oauth.Digest(cookie.Value))
		if err != nil {
			t.Fatal(err)
		}
		// Insert an already-expired synthetic challenge through normal constraints.
		// No immutable row is updated and no lifecycle trigger is disabled.
		id, err := siwe.Random()
		if err != nil {
			t.Fatal(err)
		}
		nonce, err := siwe.Random()
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		c := siwe.Challenge{ID: id, Nonce: nonce, Address: f.address, SessionReference: session.Reference, TransactionID: view.TransactionID, ClientID: f.client.ID, TenantID: f.base.scope.TenantID(), IssuedAt: now.Add(-3 * time.Minute), ExpiresAt: now.Add(-time.Minute)}
		c.Message, err = siwe.Message(c)
		if err != nil {
			t.Fatal(err)
		}
		_, err = integrationPool.Exec(context.Background(), `INSERT INTO siwe_challenges(challenge_id,nonce,message,address,tenant_id,session_reference,transaction_id,client_id,domain,uri,chain_id,issued_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'connect.wizpay.xyz','https://connect.wizpay.xyz','5042',$9,$10)`, c.ID, c.Nonce, c.Message, c.Address, c.TenantID, c.SessionReference, c.TransactionID, c.ClientID, c.IssuedAt, c.ExpiresAt)
		if err != nil {
			t.Fatal(err)
		}
		protocolStatus(t, f.browser("POST", "/browser/siwe/verify", protocolBody(t, map[string]string{"challenge_id": c.ID, "signature": siweSignature(t, c.Message)}), cookie, view.CSRF), 403)
		var consumed bool
		if err = integrationPool.QueryRow(context.Background(), `SELECT consumed_at IS NOT NULL FROM siwe_challenges WHERE challenge_id=$1`, c.ID).Scan(&consumed); err != nil || consumed {
			t.Fatal("expired challenge consumed")
		}
		if f.view(t, cookie).State != "PENDING" {
			t.Fatal("expired challenge authenticated")
		}
	})
	t.Run("expired-session", func(t *testing.T) {
		f := newProtocolFixture(t, "5042")
		cookie, view := f.start(t)
		f.offset.Store(int64(6 * time.Minute))
		protocolStatus(t, f.browser("GET", "/browser/session", "", cookie, ""), 401)
		protocolStatus(t, f.browser("POST", "/browser/consent", `{"decision":"grant"}`, cookie, view.CSRF), 403)
	})
}

func TestProtocolWrongSignerAndNetwork(t *testing.T) {
	for _, kind := range []string{"wrong-signer", "unknown-wallet", "wrong-binding-chain"} {
		t.Run(kind, func(t *testing.T) {
			chain := "5042"
			if kind == "wrong-binding-chain" {
				chain = "1"
			}
			f := newProtocolFixture(t, chain)
			cookie, view := f.start(t)
			key, err := crypto.HexToECDSA(strings.Repeat("0", 63) + "2")
			if err != nil {
				t.Fatal(err)
			}
			address := f.address
			if kind == "unknown-wallet" {
				address = crypto.PubkeyToAddress(key.PublicKey).Hex()
			}
			w := f.browser("POST", "/browser/siwe/challenge", protocolBody(t, map[string]string{"address": address}), cookie, view.CSRF)
			protocolStatus(t, w, 200)
			var c struct {
				ID      string `json:"challenge_id"`
				Message string `json:"message"`
			}
			protocolJSON(t, w, &c)
			sig := siweSignature(t, c.Message)
			if kind != "wrong-binding-chain" {
				hash := crypto.Keccak256([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len([]byte(c.Message)))), []byte(c.Message))
				b, err := crypto.Sign(hash, key)
				if err != nil {
					t.Fatal(err)
				}
				sig = "0x" + hex.EncodeToString(b)
			}
			protocolStatus(t, f.browser("POST", "/browser/siwe/verify", protocolBody(t, map[string]string{"challenge_id": c.ID, "signature": sig}), cookie, view.CSRF), 403)
			if f.view(t, cookie).State != "PENDING" {
				t.Fatal("invalid signer/network authenticated")
			}
		})
	}
}

func TestProtocolSessionCookieSubstitution(t *testing.T) {
	f := newProtocolFixture(t, "5042")
	first := f.login(t)
	second := f.login(t)
	if first.view.ClientID != second.view.ClientID || first.view.TransactionID == second.view.TransactionID || first.view.ChallengeID == second.view.ChallengeID {
		t.Fatal("substitution fixture lacks distinct same-client flows")
	}
	// Simulate a shared cookie replaced by another tab. The server returns that
	// cookie's own persistent correlation, never the first tab's challenge.
	actual := f.view(t, second.authenticated)
	if actual.TransactionID != second.view.TransactionID || actual.ChallengeID != second.challenge.ID || actual.ChallengeID == first.challenge.ID {
		t.Fatal("server substituted correlation")
	}
	protocolStatus(t, f.browser("POST", "/browser/consent", `{"decision":"grant"}`, second.authenticated, first.view.CSRF), 403)
	protocolStatus(t, f.browser("POST", "/browser/logout", "", second.authenticated, first.view.CSRF), 403)
	if f.view(t, second.authenticated).State != "AUTHENTICATED" || f.view(t, first.authenticated).State != "AUTHENTICATED" {
		t.Fatal("mismatched cleanup revoked another flow")
	}
}
