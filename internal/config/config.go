package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAppEnv     = "development"
	DefaultServerPort = 8080
	DefaultLogLevel   = "info"
)

type AuthConfig struct {
	Required      bool
	Issuer        string
	Audience      string
	PublicKeyFile string
	ClockSkew     time.Duration
}

// Config contains non-secret process configuration.
type Config struct {
	AppEnv     string
	ServerPort int
	LogLevel   string
	// AutonomousEnabled is an explicit rollout control. It defaults false and
	// does not assemble a provider or grant signing authority.
	AutonomousEnabled bool
	Auth              AuthConfig
}

func (c Config) Address() string { return ":" + strconv.Itoa(c.ServerPort) }

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
	if c.Auth.Required {
		for key, value := range map[string]string{"AUTH_ISSUER": c.Auth.Issuer, "AUTH_AUDIENCE": c.Auth.Audience, "AUTH_PUBLIC_KEY_FILE": c.Auth.PublicKeyFile} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s is required when authentication is enabled", key)
			}
		}
		if c.Auth.ClockSkew < 0 || c.Auth.ClockSkew > 5*time.Minute {
			return fmt.Errorf("AUTH_CLOCK_SKEW must be between 0 and 5m")
		}
	}
	if !c.Auth.Required && c.AppEnv == "production" {
		return fmt.Errorf("authentication is required in production")
	}
	return nil
}
