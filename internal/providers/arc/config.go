// Package arc implements the Arc chain adapter used for on-chain receipt
// verification.
//
// The surface is deliberately minimal and read-only. This package never imports
// a private key or seed phrase, never signs locally, never executes arbitrary
// contract calls, and exposes no general-purpose JSON-RPC passthrough. It reads
// transaction receipts and the current block height, and nothing else.
package arc

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// ChainIDTestnet is the Arc Testnet chain ID.
	ChainIDTestnet = "5042002"
	// RPCTestnet is the canonical Arc Testnet JSON-RPC endpoint.
	RPCTestnet = "https://rpc.testnet.arc.io"
	// ExplorerTestnet is the Arc Testnet block explorer.
	ExplorerTestnet = "https://testnet.arcscan.app"
	// NetworkTestnet is this system's network label for Arc Testnet.
	NetworkTestnet = "TESTNET"

	// NativeCurrencyDecimals is the precision of Arc's native gas currency.
	//
	// Arc uses USDC as its native gas token in an 18-decimal native
	// denomination. This value describes gas units ONLY. An ERC-20 USDC balance
	// uses its own contract-declared decimals, which are not this value. Native
	// gas units and token units must never be mixed or converted into one
	// another using this constant.
	NativeCurrencyDecimals = 18

	defaultTimeout          = 15 * time.Second
	maxTimeout              = time.Minute
	defaultMinConfirmations = 2
	maxMinConfirmations     = 64
)

// Config is the Arc chain configuration. Exactly one RPC endpoint is used: no
// fallback or alternate providers are configured, so verification evidence
// always comes from a single known source.
type Config struct {
	Enabled          bool
	ChainID          string
	Network          string
	RPCURL           string
	ExplorerURL      string
	MinConfirmations uint64
	Timeout          time.Duration
}

// LoadConfig reads Arc configuration from the environment, defaulting to the
// Arc Testnet values this phase targets. No mainnet configuration is provided.
func LoadConfig(lookup func(string) (string, bool)) (Config, error) {
	if lookup == nil {
		return Config{}, fmt.Errorf("environment lookup is required")
	}
	config := Config{
		Enabled:          boolValue(lookup, "WIZPAY_ARC_ENABLED", false),
		ChainID:          stringValue(lookup, "WIZPAY_ARC_CHAIN_ID", ChainIDTestnet),
		Network:          stringValue(lookup, "WIZPAY_ARC_NETWORK", NetworkTestnet),
		RPCURL:           stringValue(lookup, "WIZPAY_ARC_RPC_URL", RPCTestnet),
		ExplorerURL:      stringValue(lookup, "WIZPAY_ARC_EXPLORER_URL", ExplorerTestnet),
		MinConfirmations: uintValue(lookup, "WIZPAY_ARC_MIN_CONFIRMATIONS", defaultMinConfirmations),
		Timeout:          durationValue(lookup, "WIZPAY_ARC_TIMEOUT", defaultTimeout),
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
	if c.ChainID != ChainIDTestnet {
		return fmt.Errorf("Arc chain ID %q is not supported by this phase", c.ChainID)
	}
	if c.Network != NetworkTestnet {
		return fmt.Errorf("Arc network %q is not supported by this phase", c.Network)
	}
	if err := validateHTTPSURL(c.RPCURL, "RPC"); err != nil {
		return err
	}
	if err := validateHTTPSURL(c.ExplorerURL, "explorer"); err != nil {
		return err
	}
	if strings.Contains(c.RPCURL, ",") {
		return fmt.Errorf("Arc RPC URL must be a single endpoint")
	}
	if c.MinConfirmations < 1 || c.MinConfirmations > maxMinConfirmations {
		return fmt.Errorf("Arc minimum confirmations must be between 1 and %d", maxMinConfirmations)
	}
	if c.Timeout <= 0 || c.Timeout > maxTimeout {
		return fmt.Errorf("Arc timeout must be positive and at most %s", maxTimeout)
	}
	return nil
}

// Configured reports configuration availability only, never chain reachability.
func (c Config) Configured() bool { return c.Validate() == nil && c.Enabled }

func validateHTTPSURL(value, name string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("Arc %s URL must be an absolute HTTPS URL", name)
	}
	return nil
}

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

func uintValue(lookup func(string) (string, bool), key string, fallback uint64) uint64 {
	if value, found := lookup(key); found {
		if parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64); err == nil {
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
