package config

import (
	"fmt"
	"strconv"
)

const (
	DefaultAppEnv     = "development"
	DefaultServerPort = 8080
	DefaultLogLevel   = "info"
)

// Config contains the non-secret process configuration supported in Phase 1.
type Config struct {
	AppEnv     string
	ServerPort int
	LogLevel   string
}

// Address returns the HTTP listen address derived from the configured port.
func (c Config) Address() string {
	return ":" + strconv.Itoa(c.ServerPort)
}

// Validate rejects unsupported or unsafe startup configuration.
func (c Config) Validate() error {
	switch c.AppEnv {
	case "development", "test", "staging", "production":
	default:
		return fmt.Errorf("APP_ENV must be one of development, test, staging, or production")
	}

	if c.ServerPort < 1 || c.ServerPort > 65535 {
		return fmt.Errorf("SERVER_PORT must be between 1 and 65535")
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, or error")
	}

	return nil
}
