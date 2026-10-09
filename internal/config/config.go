package config

import (
	"fmt"
	"net"
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
	// ServerHost optionally restricts the listener to a loopback IP. Empty preserves existing deployment wiring.
	ServerHost string
	LogLevel   string
	// AutonomousEnabled is an explicit rollout control. It defaults false and
	// does not assemble a provider or grant signing authority.
	AutonomousEnabled  bool
	OAuthEnabled       bool
	SIWEEnabled        bool
	OnboardingTenantID string
	SIWEOrigin         string
	Auth               AuthConfig
}

func (c Config) Address() string { return net.JoinHostPort(c.ServerHost, strconv.Itoa(c.ServerPort)) }

func (c Config) Validate() error {
	if c.ServerHost != "" {
		ip := net.ParseIP(c.ServerHost)
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("SERVER_HOST must be a loopback IP when specified")
		}
	}
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
	if c.SIWEEnabled {
		if c.AppEnv != "development" && c.AppEnv != "test" {
			return fmt.Errorf("SIWE authentication is not approved for staging or production")
		}
		if !c.OAuthEnabled || !c.Auth.Required || c.AutonomousEnabled {
			return fmt.Errorf("SIWE requires authenticated read-only OAuth and disabled autonomy")
		}
		if c.SIWEOrigin != "https://connect.wizpay.xyz" || c.OnboardingTenantID == "" || strings.TrimSpace(c.OnboardingTenantID) != c.OnboardingTenantID || len(c.OnboardingTenantID) > 256 {
			return fmt.Errorf("SIWE requires the canonical origin and an explicit onboarding tenant")
		}
	}
	if c.OAuthEnabled && !c.Auth.Required {
		return fmt.Errorf("OAUTH_ENABLED requires AUTH_REQUIRED")
	}
	if c.Auth.Required && !c.OAuthEnabled {
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
