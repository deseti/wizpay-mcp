package arc_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/providers/arc"
)

// TestArcTestnetIntegration is an optional read-only Arc Testnet probe.
//
// It is skipped unless WIZPAY_ARC_INTEGRATION=1. Default `go test ./...` remains
// offline and deterministic.
//
// What it does:
//   - eth_chainId must equal 5042002
//   - eth_blockNumber must return a positive height
//
// What it does NOT do:
//   - submit transactions
//   - send raw arbitrary RPC methods
//   - touch Circle or financial APIs
func TestArcTestnetIntegration(t *testing.T) {
	if os.Getenv("WIZPAY_ARC_INTEGRATION") != "1" {
		t.Skip("set WIZPAY_ARC_INTEGRATION=1 to run Arc Testnet read-only integration checks")
	}
	config := arc.Config{
		Enabled: true, ChainID: arc.ChainIDTestnet, Network: arc.NetworkTestnet,
		RPCURL: arc.RPCTestnet, ExplorerURL: arc.ExplorerTestnet,
		MinConfirmations: 1, Timeout: 15 * time.Second,
	}
	client, err := arc.NewClient(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	chainID, blockNumber, err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("Arc Testnet health: %v", err)
	}
	if chainID != arc.ChainIDTestnet {
		t.Fatalf("chain ID = %s", chainID)
	}
	if blockNumber == 0 {
		t.Fatal("block number is zero")
	}
	t.Logf("Arc Testnet OK chain=%s head=%d", chainID, blockNumber)
}
