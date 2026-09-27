// Package circle implements the Circle User-Controlled Wallet provider
// boundary.
//
// Custody model: the user owns and controls the wallet and retains sole signing
// authority. This package never holds private keys, seed phrases, or signing
// shares, never signs, and never gains unilateral authority to move funds. It
// initiates challenges that only the user can complete.
//
// It also never creates users, never creates wallets, and never provisions
// Circle identities during financial execution.
package circle

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Blockchain is a documented Circle blockchain enum value.
type Blockchain string

const (
	arcMainnetChainID = "5042"
	arcMainnetNetwork = "MAINNET"
)

const (
	defaultBaseURL = "https://api.circle.com"
	defaultTimeout = 20 * time.Second
	maxTimeout     = 2 * time.Minute
)

// APIKey wraps the Circle API secret so it cannot be logged, printed, or
// serialized by accident. Only the outbound request builder may reveal it.
type APIKey struct {
	value string
}

func (k APIKey) Present() bool    { return k.value != "" }
func (k APIKey) reveal() string   { return k.value }
func (k APIKey) String() string   { return "[REDACTED]" }
func (k APIKey) GoString() string { return "[REDACTED]" }
func (k APIKey) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("API key must never be serialized")
}

// Config is the Circle provider configuration. The API key is secret config
// only: it is never logged, never persisted, and never placed into audit data.
type Config struct {
	Enabled    bool
	BaseURL    string
	APIKey     APIKey
	Blockchain Blockchain
	ChainID    string
	Network    string
	Timeout    time.Duration
}

// LoadConfig reads Circle configuration from the environment. A provider that
// is disabled or missing its secret is reported as unconfigured rather than
// being silently constructed in a degraded state.
func LoadConfig(lookup func(string) (string, bool)) (Config, error) {
	if lookup == nil {
		return Config{}, fmt.Errorf("environment lookup is required")
	}
	config := Config{
		Enabled: boolValue(lookup, "WIZPAY_CIRCLE_ENABLED", false),
		BaseURL: stringValue(lookup, "WIZPAY_CIRCLE_BASE_URL", defaultBaseURL),
		APIKey:  APIKey{value: strings.TrimSpace(stringValue(lookup, "WIZPAY_CIRCLE_API_KEY", ""))},
		// No Arc Mainnet execution enum is selected in Track A. A configured
		// value is retained only as unresolved metadata and never authorizes an
		// adapter.
		Blockchain: Blockchain(stringValue(lookup, "WIZPAY_CIRCLE_BLOCKCHAIN", "")),
		ChainID:    stringValue(lookup, "WIZPAY_ARC_CHAIN_ID", ""),
		Network:    stringValue(lookup, "WIZPAY_ARC_NETWORK", ""),
		Timeout:    durationValue(lookup, "WIZPAY_CIRCLE_TIMEOUT", defaultTimeout),
	}
	if !config.Enabled {
		return config, nil
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if !c.APIKey.Present() {
		return fmt.Errorf("Circle API key is required when the provider is enabled")
	}
	parsed, err := url.Parse(c.BaseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("Circle base URL must be an absolute HTTPS URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("Circle base URL must not contain a query or fragment")
	}
	if c.ChainID != arcMainnetChainID || c.Network != arcMainnetNetwork {
		return fmt.Errorf("Circle provider requires Arc Mainnet chain ID %s and network %s", arcMainnetChainID, arcMainnetNetwork)
	}
	if c.Timeout <= 0 || c.Timeout > maxTimeout {
		return fmt.Errorf("Circle timeout must be positive and at most %s", maxTimeout)
	}
	return fmt.Errorf("Circle Arc Mainnet execution is UNVERIFIED / REQUIRES OWNER DECISION and unavailable in Track A")
}

// Configured reports whether the provider can actually execute. Track A has no
// reviewed Arc Mainnet user/delegated authorization provider, so this always
// fails closed even when environment fields and an API key are present.
func (c Config) Configured() bool { return false }

func stringValue(lookup func(string) (string, bool), key, fallback string) string {
	if value, found := lookup(key); found {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return fallback
}

func boolValue(lookup func(string) (string, bool), key string, fallback bool) bool {
	if value, found := lookup(key); found {
		if parsed, err := strconv.ParseBool(strings.TrimSpace(value)); err == nil {
			return parsed
		}
	}
	return fallback
}

func durationValue(lookup func(string) (string, bool), key string, fallback time.Duration) time.Duration {
	if value, found := lookup(key); found {
		if parsed, err := time.ParseDuration(strings.TrimSpace(value)); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}
