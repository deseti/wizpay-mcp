package postgres

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxConnections = int32(10)
	defaultMinConnections = int32(1)
	defaultConnectTimeout = 5 * time.Second
	defaultQueryTimeout   = 5 * time.Second
)

type Config struct {
	URL            string
	MigrationURL   string
	MaxConnections int32
	MinConnections int32
	ConnectTimeout time.Duration
	QueryTimeout   time.Duration
}

type LookupEnv func(string) (string, bool)

func LoadConfig(lookup LookupEnv) (Config, error) {
	if lookup == nil {
		return Config{}, fmt.Errorf("database configuration lookup is required")
	}
	cfg := Config{MaxConnections: defaultMaxConnections, MinConnections: defaultMinConnections, ConnectTimeout: defaultConnectTimeout, QueryTimeout: defaultQueryTimeout}
	if value, ok := lookup("DATABASE_URL"); ok {
		cfg.URL = value
	}
	if value, ok := lookup("DATABASE_MIGRATION_URL"); ok {
		cfg.MigrationURL = value
	} else {
		cfg.MigrationURL = cfg.URL
	}
	var err error
	if cfg.MaxConnections, err = int32Value(lookup, "DATABASE_MAX_CONNECTIONS", cfg.MaxConnections); err != nil {
		return Config{}, err
	}
	if cfg.MinConnections, err = int32Value(lookup, "DATABASE_MIN_CONNECTIONS", cfg.MinConnections); err != nil {
		return Config{}, err
	}
	if cfg.ConnectTimeout, err = durationValue(lookup, "DATABASE_CONNECT_TIMEOUT", cfg.ConnectTimeout); err != nil {
		return Config{}, err
	}
	if cfg.QueryTimeout, err = durationValue(lookup, "DATABASE_QUERY_TIMEOUT", cfg.QueryTimeout); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid database configuration")
	}
	return cfg, nil
}

func (c Config) Validate() error {
	migrationURL := c.MigrationURL
	if migrationURL == "" {
		migrationURL = c.URL
	}
	if !validPostgresURL(c.URL) || !validPostgresURL(migrationURL) {
		return fmt.Errorf("DATABASE_URL must be a complete PostgreSQL connection URL")
	}
	if c.MaxConnections < 1 || c.MaxConnections > 100 {
		return fmt.Errorf("database maximum connections must be between 1 and 100")
	}
	if c.MinConnections < 0 || c.MinConnections > c.MaxConnections {
		return fmt.Errorf("database minimum connections must be between 0 and maximum connections")
	}
	if c.ConnectTimeout <= 0 || c.ConnectTimeout > time.Minute {
		return fmt.Errorf("database connect timeout must be between 1ns and 1m")
	}
	if c.QueryTimeout <= 0 || c.QueryTimeout > time.Minute {
		return fmt.Errorf("database query timeout must be between 1ns and 1m")
	}
	return nil
}

func validPostgresURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "postgres" || parsed.Scheme == "postgresql") && parsed.Host != "" && parsed.User != nil && parsed.Path != "" && parsed.Path != "/"
}

func (c Config) MigrationConfig() Config {
	if c.MigrationURL != "" {
		c.URL = c.MigrationURL
	}
	c.MaxConnections, c.MinConnections = 1, 0
	return c
}

func int32Value(lookup LookupEnv, key string, fallback int32) (int32, error) {
	value, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be a base-10 integer", key)
	}
	return int32(parsed), nil
}
func durationValue(lookup LookupEnv, key string, fallback time.Duration) (time.Duration, error) {
	value, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("%s must be a Go duration", key)
	}
	return parsed, nil
}
