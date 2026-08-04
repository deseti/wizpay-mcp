package arc

import (
	"strings"
	"testing"
	"time"
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
	if config.Enabled {
		t.Fatalf("Arc must be disabled unless explicitly enabled")
	}
	if config.Configured() {
		t.Fatalf("a disabled Arc config must not report configured")
	}
	// Defaults must still describe the Arc Testnet target.
	if config.ChainID != ChainIDTestnet || config.Network != NetworkTestnet || config.RPCURL != RPCTestnet {
		t.Fatalf("defaults must target Arc Testnet, got %+v", config)
	}
	if config.MinConfirmations != 1 {
		t.Fatalf("default MinConfirmations must be 1 for Arc BFT finality, got %d", config.MinConfirmations)
	}
}

func TestLoadConfigEnabledValid(t *testing.T) {
	config, err := LoadConfig(lookupFrom(map[string]string{"WIZPAY_ARC_ENABLED": "true"}))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !config.Configured() {
		t.Fatalf("an enabled default config must be configured")
	}
	if config.MinConfirmations != 1 {
		t.Fatalf("enabled default MinConfirmations must be 1, got %d", config.MinConfirmations)
	}
	if config.RPCURL != "https://rpc.testnet.arc.io" {
		t.Fatalf("canonical RPC must remain .io, got %q", config.RPCURL)
	}
}

func TestCanonicalRPCConstants(t *testing.T) {
	if RPCTestnet != "https://rpc.testnet.arc.io" {
		t.Fatalf("RPCTestnet = %q", RPCTestnet)
	}
	if strings.Contains(RPCTestnet, "arc.network") {
		t.Fatal("canonical RPC must not use rpc.testnet.arc.network")
	}
	if ChainIDTestnet != "5042002" {
		t.Fatalf("ChainIDTestnet = %q", ChainIDTestnet)
	}
}

func TestLoadConfigRejectsUnsupportedChain(t *testing.T) {
	_, err := LoadConfig(lookupFrom(map[string]string{
		"WIZPAY_ARC_ENABLED":  "true",
		"WIZPAY_ARC_CHAIN_ID": "1",
	}))
	if err == nil {
		t.Fatalf("a non-Arc-Testnet chain must be rejected")
	}
}

func TestValidateRejects(t *testing.T) {
	base := func() Config {
		return Config{
			Enabled: true, ChainID: ChainIDTestnet, Network: NetworkTestnet,
			RPCURL: RPCTestnet, ExplorerURL: ExplorerTestnet, MinConfirmations: 1, Timeout: 15 * time.Second,
		}
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("baseline config must be valid: %v", err)
	}
	cases := map[string]func(*Config){
		"non-https rpc":          func(c *Config) { c.RPCURL = "http://rpc.testnet.arc.io" },
		"multi-endpoint rpc":     func(c *Config) { c.RPCURL = RPCTestnet + "," + RPCTestnet },
		"non-canonical rpc host": func(c *Config) { c.RPCURL = "https://rpc.testnet.arc.network" },
		"zero confirmations":     func(c *Config) { c.MinConfirmations = 0 },
		"excess confirmations":   func(c *Config) { c.MinConfirmations = 1_000 },
		"non-positive timeout":   func(c *Config) { c.Timeout = 0 },
		"excessive timeout":      func(c *Config) { c.Timeout = time.Hour },
		"unsupported network":    func(c *Config) { c.Network = "MAINNET" },
	}
	for name, mutate := range cases {
		config := base()
		mutate(&config)
		if err := config.Validate(); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

func TestValidateAcceptsOneConfirmation(t *testing.T) {
	config := Config{
		Enabled: true, ChainID: ChainIDTestnet, Network: NetworkTestnet,
		RPCURL: RPCTestnet, ExplorerURL: ExplorerTestnet, MinConfirmations: 1, Timeout: 15 * time.Second,
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("MinConfirmations=1 must be valid: %v", err)
	}
}
