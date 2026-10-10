package main

import (
	"github.com/deseti/wizpay-mcp/internal/siwe"
	"strings"
	"testing"
	"time"
)

func TestRestrictedEphemeralSigner(t *testing.T) {
	now := time.Now().UTC()
	c := siwe.Challenge{Address: "0x0000000000000000000000000000000000000001", Nonce: strings.Repeat("a", 64), IssuedAt: now, ExpiresAt: now.Add(time.Minute)}
	m, err := siwe.Message(c)
	if err != nil || !eligible(m, c.Address, now) {
		t.Fatal("valid restricted message rejected")
	}
	for _, altered := range []string{m + "\n", strings.Replace(m, "5042", "1", 1), strings.Replace(m, "connect.wizpay.xyz", "attacker.example", 1), "financial signature"} {
		if eligible(altered, c.Address, now) {
			t.Fatal("unexpected message accepted")
		}
	}
	if eligible(m, c.Address, now.Add(2*time.Minute)) {
		t.Fatal("expired message accepted")
	}
}
