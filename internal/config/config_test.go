package config

import (
	"strings"
	"testing"
)

func TestLoadWithLookupDefaults(t *testing.T) {
	cfg, err := LoadWithLookup(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("LoadWithLookup() error = %v", err)
	}
	if cfg.AppEnv != DefaultAppEnv || cfg.ServerPort != DefaultServerPort || cfg.LogLevel != DefaultLogLevel || cfg.Auth.Required {
		t.Fatalf("LoadWithLookup() = %+v, want defaults", cfg)
	}
}

func TestLoadWithLookupOverrides(t *testing.T) {
	values := map[string]string{"APP_ENV": "production", "SERVER_PORT": "9090", "LOG_LEVEL": "WARN", "AUTH_REQUIRED": "true", "AUTH_ISSUER": "https://issuer.example", "AUTH_AUDIENCE": "wizpay-mcp", "AUTH_PUBLIC_KEY_FILE": "/run/secrets/auth-public.pem"}
	cfg, err := LoadWithLookup(func(key string) (string, bool) { value, ok := values[key]; return value, ok })
	if err != nil {
		t.Fatalf("LoadWithLookup() error = %v", err)
	}
	if cfg.AppEnv != "production" || cfg.ServerPort != 9090 || cfg.LogLevel != "warn" || !cfg.Auth.Required {
		t.Fatalf("LoadWithLookup() = %+v", cfg)
	}
}

func TestLoadWithLookupRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		values    map[string]string
		wantError string
	}{
		{"environment", map[string]string{"APP_ENV": "unknown"}, "APP_ENV"},
		{"port format", map[string]string{"SERVER_PORT": "not-a-port"}, "base-10 integer"},
		{"port range", map[string]string{"SERVER_PORT": "70000"}, "between 1 and 65535"},
		{"log level", map[string]string{"LOG_LEVEL": "trace"}, "LOG_LEVEL"},
		{"missing auth issuer", map[string]string{"AUTH_REQUIRED": "true", "AUTH_AUDIENCE": "audience", "AUTH_PUBLIC_KEY_FILE": "key.pem"}, "AUTH_ISSUER"},
		{"bad auth skew", map[string]string{"AUTH_REQUIRED": "true", "AUTH_ISSUER": "issuer", "AUTH_AUDIENCE": "audience", "AUTH_PUBLIC_KEY_FILE": "key.pem", "AUTH_CLOCK_SKEW": "bad"}, "AUTH_CLOCK_SKEW"},
		{"bad auth required", map[string]string{"AUTH_REQUIRED": "tru"}, "AUTH_REQUIRED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadWithLookup(func(key string) (string, bool) { value, ok := test.values[key]; return value, ok })
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestValidateAddress(t *testing.T) {
	cfg := Config{AppEnv: "test", ServerPort: 8081, LogLevel: "debug"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := cfg.Address(); got != ":8081" {
		t.Fatalf("Address() = %q", got)
	}
}
