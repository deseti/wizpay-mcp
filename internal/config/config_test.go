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

	if cfg.AppEnv != DefaultAppEnv || cfg.ServerPort != DefaultServerPort || cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("LoadWithLookup() = %+v, want defaults", cfg)
	}
}

func TestLoadWithLookupOverrides(t *testing.T) {
	values := map[string]string{
		"APP_ENV":     "production",
		"SERVER_PORT": "9090",
		"LOG_LEVEL":   "WARN",
	}
	cfg, err := LoadWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatalf("LoadWithLookup() error = %v", err)
	}

	if cfg.AppEnv != "production" || cfg.ServerPort != 9090 || cfg.LogLevel != "warn" {
		t.Fatalf("LoadWithLookup() = %+v, want production overrides", cfg)
	}
}

func TestLoadWithLookupRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		values    map[string]string
		wantError string
	}{
		{name: "environment", values: map[string]string{"APP_ENV": "unknown"}, wantError: "APP_ENV"},
		{name: "port format", values: map[string]string{"SERVER_PORT": "not-a-port"}, wantError: "base-10 integer"},
		{name: "port range", values: map[string]string{"SERVER_PORT": "70000"}, wantError: "between 1 and 65535"},
		{name: "log level", values: map[string]string{"LOG_LEVEL": "trace"}, wantError: "LOG_LEVEL"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadWithLookup(func(key string) (string, bool) {
				value, ok := test.values[key]
				return value, ok
			})
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("LoadWithLookup() error = %v, want containing %q", err, test.wantError)
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
		t.Fatalf("Address() = %q, want %q", got, ":8081")
	}
}
