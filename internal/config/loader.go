package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type LookupEnv func(string) (string, bool)

func Load() (Config, error) { return LoadWithLookup(os.LookupEnv) }

func LoadWithLookup(lookup LookupEnv) (Config, error) {
	if lookup == nil {
		return Config{}, fmt.Errorf("configuration lookup is required")
	}
	authRequired, err := boolValue(lookup, "AUTH_REQUIRED", false)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		AppEnv: stringValue(lookup, "APP_ENV", DefaultAppEnv), ServerPort: DefaultServerPort,
		LogLevel: strings.ToLower(stringValue(lookup, "LOG_LEVEL", DefaultLogLevel)),
		Auth:     AuthConfig{Required: authRequired, Issuer: stringValue(lookup, "AUTH_ISSUER", ""), Audience: stringValue(lookup, "AUTH_AUDIENCE", ""), PublicKeyFile: stringValue(lookup, "AUTH_PUBLIC_KEY_FILE", ""), ClockSkew: durationValue(lookup, "AUTH_CLOCK_SKEW", 30*time.Second)},
	}
	if value, ok := lookup("SERVER_PORT"); ok {
		port, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("SERVER_PORT must be a base-10 integer")
		}
		cfg.ServerPort = port
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}
func stringValue(lookup LookupEnv, key, fallback string) string {
	if value, ok := lookup(key); ok {
		return value
	}
	return fallback
}
func boolValue(lookup LookupEnv, key string, fallback bool) (bool, error) {
	value, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true, nil
	case "0", "false", "no":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean", key)
	}
}
func durationValue(lookup LookupEnv, key string, fallback time.Duration) time.Duration {
	value, ok := lookup(key)
	if !ok {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return -1
	}
	return parsed
}
