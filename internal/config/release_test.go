package config

import (
	"errors"
	"strings"
	"testing"
)

func TestProductionReadOnlyBoundary(t *testing.T) {
	c := Config{AppEnv: "production", ServerPort: 8080, LogLevel: "info", OAuthEnabled: true, Auth: AuthConfig{Required: true}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Config){func(c *Config) { c.OAuthEnabled = false }, func(c *Config) { c.AutonomousEnabled = true }, func(c *Config) { c.MigrateOnStart = true }, func(c *Config) { c.Auth.Required = false }, func(c *Config) { c.SIWEEnabled = true }} {
		bad := c
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("unsafe production configuration accepted")
		}
	}
}
func TestDatabaseSecretLookup(t *testing.T) {
	lookup := func(k string) (string, bool) { return "/run/secrets/database", k == "DATABASE_URL_FILE" }
	l, err := SecretLookup(lookup, func(string) ([]byte, error) { return []byte("postgres://synthetic\n"), nil })
	if err != nil {
		t.Fatal(err)
	}
	v, ok := l("DATABASE_URL")
	if !ok || v != "postgres://synthetic" {
		t.Fatal("file not loaded")
	}
	_, err = SecretLookup(lookup, func(string) ([]byte, error) { return nil, errors.New("SECRET_CONTENT") })
	if err == nil || strings.Contains(err.Error(), "SECRET_CONTENT") {
		t.Fatal("unsafe secret diagnostic")
	}
	_, err = SecretLookup(func(k string) (string, bool) { return "value", k == "DATABASE_URL" || k == "DATABASE_URL_FILE" }, func(string) ([]byte, error) { t.Fatal("conflict read file"); return nil, nil })
	if err == nil {
		t.Fatal("conflict accepted")
	}
}
