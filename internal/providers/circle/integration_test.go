package circle_test

import (
	"testing"

	"github.com/deseti/wizpay-mcp/internal/providers/circle"
)

// Track A deliberately has no Circle Arc Mainnet integration harness. Even a
// fully populated environment-shaped configuration must fail before any
// client, health probe, challenge, or transaction path can be constructed.
func TestCircleArcMainnetExecutionUnavailableInTrackA(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"WIZPAY_CIRCLE_ENABLED":    "true",
			"WIZPAY_CIRCLE_API_KEY":    "test-only-placeholder",
			"WIZPAY_CIRCLE_BLOCKCHAIN": "ARC",
			"WIZPAY_ARC_CHAIN_ID":      "5042",
			"WIZPAY_ARC_NETWORK":       "MAINNET",
		}
		value, found := values[key]
		return value, found
	}
	config, err := circle.LoadConfig(lookup)
	if err == nil || config.Configured() {
		t.Fatalf("unreviewed Mainnet Circle path became available: config=%#v err=%v", config, err)
	}
}
