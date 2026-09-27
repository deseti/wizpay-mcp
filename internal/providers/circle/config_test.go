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

func TestLoadConfigEnabledIsUnavailableInTrackA(t *testing.T) {
	_, err := LoadConfig(lookupFrom(map[string]string{
		"WIZPAY_CIRCLE_ENABLED":    "true",
		"WIZPAY_CIRCLE_API_KEY":    "secret-key",
		"WIZPAY_CIRCLE_BLOCKCHAIN": "ARC",
		"WIZPAY_ARC_CHAIN_ID":      arc.ChainIDMainnet,
		"WIZPAY_ARC_NETWORK":       arc.NetworkMainnet,
	}))
	if err == nil {
		t.Fatalf("Circle Arc Mainnet execution must remain unavailable in Track A")
	}
}

func TestValidateRejects(t *testing.T) {
	base := func() Config {
		return Config{
			Enabled: true, BaseURL: defaultBaseURL, APIKey: APIKey{value: "secret"},
			Blockchain: Blockchain("ARC"), ChainID: arc.ChainIDMainnet, Network: arc.NetworkMainnet,
			Timeout: 20 * time.Second,
		}
	}
	if err := base().Validate(); err == nil {
		t.Fatalf("fully populated Mainnet config must remain unavailable")
	}
	cases := map[string]func(*Config){
		"non-https base url":   func(c *Config) { c.BaseURL = "http://api.circle.com" },
		"base url with query":  func(c *Config) { c.BaseURL = "https://api.circle.com?x=1" },
		"missing chain id":     func(c *Config) { c.ChainID = "" },
		"missing network":      func(c *Config) { c.Network = "" },
		"Arc Testnet chain":    func(c *Config) { c.ChainID = "5042002" },
		"Arc Testnet network":  func(c *Config) { c.Network = "TESTNET" },
		"non-positive timeout": func(c *Config) { c.Timeout = 0 },
		"excessive timeout":    func(c *Config) { c.Timeout = time.Hour },
	}
	for name, mutate := range cases {
		config := base()
		mutate(&config)
		if err := config.Validate(); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

func TestConfiguredAlwaysFailsClosedForMainnet(t *testing.T) {
	config := Config{
		Enabled: true, BaseURL: defaultBaseURL, APIKey: APIKey{value: "secret"},
		Blockchain: Blockchain("ARC"), ChainID: arc.ChainIDMainnet, Network: arc.NetworkMainnet,
		Timeout: 20 * time.Second,
	}
	if config.Configured() {
		t.Fatal("unreviewed Circle Arc Mainnet execution must never report configured")
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
