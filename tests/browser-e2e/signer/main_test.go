package main

import (
	"encoding/hex"
	"github.com/deseti/wizpay-mcp/internal/siwe"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
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

func TestBothPurposesRetainRealSignatureVerification(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal("synthetic key generation failed")
	}
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	now := time.Now().UTC()
	for _, purpose := range []string{"ENROLL", "AUTHENTICATE"} {
		c := siwe.Challenge{Address: address, Nonce: strings.Repeat("a", 64), IssuedAt: now, ExpiresAt: now.Add(time.Minute), Purpose: purpose}
		message, err := siwe.Message(c)
		if err != nil || !eligible(message, address, now) {
			t.Fatal("restricted purpose rejected")
		}
		signature, err := crypto.Sign(accounts.TextHash([]byte(message)), key)
		if err != nil || siwe.Verify(message, address, "0x"+hex.EncodeToString(signature)) != nil {
			t.Fatal("real signature verification failed")
		}
		if siwe.Verify(message+" altered", address, "0x"+hex.EncodeToString(signature)) == nil {
			t.Fatal("altered message verified")
		}
		if eligible(strings.Replace(message, "This does not", "This authorizes", 1), address, now) {
			t.Fatal("financial statement accepted")
		}
	}
}

func TestSyntheticFixtureStagesWithoutManufacturingAuthority(t *testing.T) {
	f, err := newFixture(strings.Repeat("a", 64), "0x"+strings.Repeat("1", 40), "0x"+strings.Repeat("2", 40))
	if err != nil {
		t.Fatal("synthetic fixture rejected")
	}
	for _, required := range []string{"current_database() <> 'wizpay_mcp_browser_e2e'", "NOLOGIN NOSUPERUSER", "SET SESSION AUTHORIZATION", "admin_onboarding('STAGE'", "status='PENDING'", "version=1", "status='UNKNOWN'", "status='INACTIVE'"} {
		if !strings.Contains(f.SQL, required) {
			t.Fatal("staging boundary missing")
		}
	}
	for _, forbidden := range []string{"INSERT INTO wallet_bindings", "INSERT INTO identities", "INSERT INTO tenants", "INSERT INTO wallet_enrollment_proofs", "UPDATE wallet_bindings", "SELECT admin_activate_wallet(", "test-only-owner-review", "PRIVATE KEY", "password"} {
		if strings.Contains(f.SQL, forbidden) {
			t.Fatal("fixture manufactures authority or credentials")
		}
	}
	if len(f.Owner) > 63 || len(f.OtherUser) > 128 {
		t.Fatal("invalid PostgreSQL identifier length")
	}
	for _, id := range []string{"", "a' injected", strings.Repeat("z", 64)} {
		if _, err := newFixture(id, f.Address, f.OtherAddress); err == nil {
			t.Fatal("unsafe fixture reference accepted")
		}
	}
	if _, err := newFixture(f.ID, f.Address, f.Address); err == nil {
		t.Fatal("duplicate wallet accepted")
	}
}
