package main

import (
	"strings"
	"testing"
	"time"
)

func TestInputFailsClosed(t *testing.T) {
	valid := `{"action":"REVOKE_TENANT","request_id":"request-1","tenant_id":"tenant-1","review_reference":"review-1","expires_at":"` + time.Now().Add(time.Minute).UTC().Format(time.RFC3339) + `"}`
	if _, err := decode(strings.NewReader(valid)); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{valid + `{}`, strings.Replace(valid, `"action":`, `"signature":"not-accepted","action":`, 1), strings.Replace(valid, "REVOKE_TENANT", "ACTIVATE", 1), strings.Repeat("x", 8194)} {
		if _, err := decode(strings.NewReader(input)); err == nil {
			t.Fatal("unsafe input accepted")
		}
	}
}

func TestAdministrationTLSCannotFallback(t *testing.T) {
	for _, mode := range []string{"disable", "allow", "prefer", "require", "verify-ca"} {
		if _, err := connectionConfig("postgres://operator@db.invalid/wizpay?sslmode=" + mode); err == nil {
			t.Fatalf("accepted TLS mode %s", mode)
		}
	}
	cfg, err := connectionConfig("postgres://operator@db.invalid/wizpay?sslmode=verify-full")
	if err != nil || cfg.TLSConfig.ServerName != "db.invalid" {
		t.Fatal("verified canonical database configuration rejected", err)
	}
}
