package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// LookupEnv matches os.LookupEnv and makes configuration loading deterministic
// in tests without mutating process-global environment variables.
type LookupEnv func(string) (string, bool)

// Load reads and validates configuration from the process environment.
func Load() (Config, error) {
	return LoadWithLookup(os.LookupEnv)
}

// LoadWithLookup reads configuration through lookup and applies defaults only
// when a variable is absent.
func LoadWithLookup(lookup LookupEnv) (Config, error) {
	if lookup == nil {
		return Config{}, fmt.Errorf("configuration lookup is required")
	}

	cfg := Config{
		AppEnv:     stringValue(lookup, "APP_ENV", DefaultAppEnv),
		ServerPort: DefaultServerPort,
		LogLevel:   strings.ToLower(stringValue(lookup, "LOG_LEVEL", DefaultLogLevel)),
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
