package circle

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/providers/arc"
)

func lookupFrom(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, found := values[key]
		return value, found
	}
}

func TestLoadConfigDisabledByDefault(t *testing.T) {
	config, err := LoadConfig(lookupFrom(nil))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if config.Configured() {
		t.Fatalf("Circle must be unconfigured by default")
	}
}

func TestLoadConfigEnabledRequiresAPIKey(t *testing.T) {
	_, err := LoadConfig(lookupFrom(map[string]string{
		"WIZPAY_CIRCLE_ENABLED": "true",
		"WIZPAY_ARC_CHAIN_ID":   arc.ChainIDTestnet,
		"WIZPAY_ARC_NETWORK":    arc.NetworkTestnet,
	}))
	if err == nil {
		t.Fatalf("an enabled provider without an API key must be rejected")
	}
}

func TestLoadConfigEnabledValid(t *testing.T) {
	config, err := LoadConfig(lookupFrom(map[string]string{
		"WIZPAY_CIRCLE_ENABLED": "true",
		"WIZPAY_CIRCLE_API_KEY": "secret-key",
		"WIZPAY_ARC_CHAIN_ID":   arc.ChainIDTestnet,
		"WIZPAY_ARC_NETWORK":    arc.NetworkTestnet,
	}))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !config.Configured() {
		t.Fatalf("a fully specified enabled provider must be configured")
	}
}

func TestValidateRejects(t *testing.T) {
	base := func() Config {
		return Config{
			Enabled: true, BaseURL: defaultBaseURL, APIKey: APIKey{value: "secret"},
			Blockchain: BlockchainArcTestnet, ChainID: arc.ChainIDTestnet, Network: arc.NetworkTestnet,
			Timeout: 20 * time.Second,
		}
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("baseline config must be valid: %v", err)
	}
	cases := map[string]func(*Config){
		"non-https base url":     func(c *Config) { c.BaseURL = "http://api.circle.com" },
		"base url with query":    func(c *Config) { c.BaseURL = "https://api.circle.com?x=1" },
		"unsupported blockchain": func(c *Config) { c.Blockchain = "ETH-MAINNET" },
		"missing chain id":       func(c *Config) { c.ChainID = "" },
		"missing network":        func(c *Config) { c.Network = "" },
		"non-positive timeout":   func(c *Config) { c.Timeout = 0 },
		"excessive timeout":      func(c *Config) { c.Timeout = time.Hour },
	}
	for name, mutate := range cases {
		config := base()
		mutate(&config)
		if err := config.Validate(); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

func TestAPIKeyNeverLeaks(t *testing.T) {
	key := APIKey{value: "super-secret"}
	if key.String() != "[REDACTED]" || key.GoString() != "[REDACTED]" {
		t.Fatalf("API key must render redacted")
	}
	if _, err := json.Marshal(key); err == nil {
		t.Fatalf("API key must refuse serialization")
	}
	if !key.Present() {
		t.Fatalf("a set key must report present")
	}
}
