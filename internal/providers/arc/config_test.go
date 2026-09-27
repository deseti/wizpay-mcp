package arc

import (
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
	// Defaults must describe the Arc Mainnet target.
	if config.ChainID != ChainIDMainnet || config.Network != NetworkMainnet || config.RPCURL != RPCMainnet {
		t.Fatalf("defaults must target Arc Mainnet, got %+v", config)
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
	if config.RPCURL != "https://rpc.mainnet.arc.io" {
		t.Fatalf("canonical RPC must remain .io, got %q", config.RPCURL)
	}
}

func TestCanonicalRPCConstants(t *testing.T) {
	if RPCMainnet != "https://rpc.mainnet.arc.io" {
		t.Fatalf("RPCMainnet = %q", RPCMainnet)
	}
	if ChainIDMainnet != "5042" {
		t.Fatalf("ChainIDMainnet = %q", ChainIDMainnet)
	}
}

func TestLoadConfigRejectsUnsupportedChain(t *testing.T) {
	_, err := LoadConfig(lookupFrom(map[string]string{
		"WIZPAY_ARC_ENABLED":  "true",
		"WIZPAY_ARC_CHAIN_ID": "1",
	}))
	if err == nil {
		t.Fatalf("a non-Arc-Mainnet chain must be rejected")
	}
}

func TestLoadConfigRejectsArcTestnet(t *testing.T) {
	_, err := LoadConfig(lookupFrom(map[string]string{
		"WIZPAY_ARC_ENABLED": "true", "WIZPAY_ARC_CHAIN_ID": "5042002",
		"WIZPAY_ARC_NETWORK": "TESTNET", "WIZPAY_ARC_RPC_URL": "https://rpc.testnet.arc.io",
		"WIZPAY_ARC_EXPLORER_URL": "https://explorer.testnet.arc.io",
	}))
	if err == nil {
		t.Fatal("Arc Testnet must not pass Mainnet validation")
	}
}

func TestValidateRejects(t *testing.T) {
	base := func() Config {
		return Config{
			Enabled: true, ChainID: ChainIDMainnet, Network: NetworkMainnet,
			RPCURL: RPCMainnet, ExplorerURL: ExplorerMainnet, MinConfirmations: 1, Timeout: 15 * time.Second,
		}
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("baseline config must be valid: %v", err)
	}
	cases := map[string]func(*Config){
		"non-https rpc":          func(c *Config) { c.RPCURL = "http://rpc.testnet.arc.io" },
		"multi-endpoint rpc":     func(c *Config) { c.RPCURL = RPCMainnet + "," + RPCMainnet },
		"non-canonical rpc host": func(c *Config) { c.RPCURL = "https://rpc.testnet.arc.network" },
		"zero confirmations":     func(c *Config) { c.MinConfirmations = 0 },
		"excess confirmations":   func(c *Config) { c.MinConfirmations = 1_000 },
		"non-positive timeout":   func(c *Config) { c.Timeout = 0 },
		"excessive timeout":      func(c *Config) { c.Timeout = time.Hour },
		"unsupported network":    func(c *Config) { c.Network = "TESTNET" },
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
		Enabled: true, ChainID: ChainIDMainnet, Network: NetworkMainnet,
		RPCURL: RPCMainnet, ExplorerURL: ExplorerMainnet, MinConfirmations: 1, Timeout: 15 * time.Second,
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("MinConfirmations=1 must be valid: %v", err)
	}
}
