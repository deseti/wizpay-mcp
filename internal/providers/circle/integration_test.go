package circle_test

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/providers/circle"
)

// TestCircleSandboxIntegration is an optional non-financial Circle probe.
//
// It is skipped unless:
//
//	WIZPAY_CIRCLE_INTEGRATION=1
//	WIZPAY_CIRCLE_API_KEY is set
//
// Default `go test ./...` remains offline and never requires secrets.
//
// What it does:
//   - GET /v1/ping (or host reachability equivalent) with API key only
//
// What it does NOT do:
//   - create wallets
//   - transfer tokens
//   - complete challenges
//   - use user tokens
//   - move funds
func TestCircleSandboxIntegration(t *testing.T) {
	if os.Getenv("WIZPAY_CIRCLE_INTEGRATION") != "1" {
		t.Skip("set WIZPAY_CIRCLE_INTEGRATION=1 and WIZPAY_CIRCLE_API_KEY to run Circle non-financial integration checks")
	}
	apiKey := os.Getenv("WIZPAY_CIRCLE_API_KEY")
	if apiKey == "" {
		t.Skip("WIZPAY_CIRCLE_API_KEY is required for Circle integration checks")
	}
	baseURL := os.Getenv("WIZPAY_CIRCLE_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.circle.com"
	}
	config := circle.Config{
		Enabled: true, BaseURL: baseURL, APIKey: mustAPIKey(t, apiKey),
		Blockchain: circle.BlockchainArcTestnet, ChainID: "5042002", Network: "TESTNET",
		Timeout: 20 * time.Second,
	}
	checker, err := circle.NewHealthChecker(config, &http.Client{Timeout: 20 * time.Second}, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	health := checker.Check(ctx)
	t.Logf("Circle integration health status=%s detail=%s", health.Status, health.Detail)
	if health.Status == "UNAVAILABLE" {
		t.Fatalf("Circle sandbox/testnet host unavailable: %s", health.Detail)
	}
}

func mustAPIKey(t *testing.T, value string) circle.APIKey {
	t.Helper()
	// Construct via LoadConfig shape: enabled config needs present key.
	// APIKey is a named type with unexported field; use LoadConfig path.
	lookup := func(key string) (string, bool) {
		switch key {
		case "WIZPAY_CIRCLE_ENABLED":
			return "true", true
		case "WIZPAY_CIRCLE_API_KEY":
			return value, true
		case "WIZPAY_ARC_CHAIN_ID":
			return "5042002", true
		case "WIZPAY_ARC_NETWORK":
			return "TESTNET", true
		default:
			return "", false
		}
	}
	config, err := circle.LoadConfig(lookup)
	if err != nil {
		t.Fatal(err)
	}
	return config.APIKey
}
