package postgres

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)) }

func TestLoadConfigDefaultsAndValidation(t *testing.T) {
	values := map[string]string{"DATABASE_URL": "postgres://user:password@localhost:5432/wizpay?sslmode=disable"}
	cfg, err := LoadConfig(func(key string) (string, bool) { value, ok := values[key]; return value, ok })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConnections != defaultMaxConnections || cfg.QueryTimeout != defaultQueryTimeout {
		t.Fatalf("defaults = %#v", cfg)
	}
	if cfg.MigrationURL != cfg.URL {
		t.Fatalf("default migration URL does not match application URL")
	}
	values["DATABASE_MIGRATION_URL"] = "postgres://migration:password@localhost:5432/wizpay?sslmode=disable"
	if cfg, err = LoadConfig(func(key string) (string, bool) { value, ok := values[key]; return value, ok }); err != nil || cfg.MigrationURL == cfg.URL {
		t.Fatalf("separate migration URL = %#v, %v", cfg, err)
	}
	values["DATABASE_QUERY_TIMEOUT"] = "0s"
	if _, err = LoadConfig(func(key string) (string, bool) { value, ok := values[key]; return value, ok }); err == nil {
		t.Fatal("zero query timeout accepted")
	}
}
func TestConfigErrorsDoNotLeakDatabaseURL(t *testing.T) {
	secretURL := "postgres://user:super-secret-password@/missing-host"
	_, err := LoadConfig(func(key string) (string, bool) {
		if key == "DATABASE_URL" {
			return secretURL, true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("invalid URL accepted")
	}
	if strings.Contains(err.Error(), "super-secret-password") {
		t.Fatalf("configuration error leaked credential: %v", err)
	}
}
func TestOpenRejectsCancelledContextWithoutCredentialLeak(t *testing.T) {
	cfg := Config{URL: "postgres://user:super-secret-password@127.0.0.1:1/wizpay", MaxConnections: 1, ConnectTimeout: time.Millisecond, QueryTimeout: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Open(ctx, cfg, testLogger())
	if err == nil {
		t.Fatal("cancelled open succeeded")
	}
	if strings.Contains(err.Error(), "super-secret-password") {
		t.Fatalf("open error leaked credential: %v", err)
	}
}
