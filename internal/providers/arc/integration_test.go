package arc_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/providers/arc"
)

// TestArcMainnetIntegration is an optional read-only Arc Mainnet probe.
//
// It is skipped unless WIZPAY_ARC_INTEGRATION=1. Default `go test ./...` remains
// offline and deterministic.
//
// What it does:
//   - eth_chainId must equal 5042
//   - eth_blockNumber must return a positive height
//   - eth_getCode must return non-empty code for both canonical deployments
//
// What it does NOT do:
//   - submit transactions
//   - send raw arbitrary RPC methods
//   - touch Circle or financial APIs
func TestArcMainnetIntegration(t *testing.T) {
	if os.Getenv("WIZPAY_ARC_INTEGRATION") != "1" {
		t.Skip("set WIZPAY_ARC_INTEGRATION=1 to run Arc Mainnet read-only integration checks")
	}
	config := arc.Config{
		Enabled: true, ChainID: arc.ChainIDMainnet, Network: arc.NetworkMainnet,
		RPCURL: arc.RPCMainnet, ExplorerURL: arc.ExplorerMainnet,
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
		t.Fatalf("Arc Mainnet health: %v", err)
	}
	if chainID != arc.ChainIDMainnet {
		t.Fatalf("chain ID = %s", chainID)
	}
	if blockNumber == 0 {
		t.Fatal("block number is zero")
	}
	for _, id := range []contracts.ContractID{contracts.ContractWizPayPayroll, contracts.ContractWizPaySwapExecutor} {
		attestation, err := client.AttestDeploymentCode(ctx, id, contracts.RegistryVersion)
		if err != nil {
			t.Fatalf("%s code attestation: %v", id, err)
		}
		if attestation.CodeSize == 0 || attestation.CodeHash == "" {
			t.Fatalf("%s has empty code attestation", id)
		}
		t.Logf("%s address=%s code_size=%d code_hash=%s", id, attestation.Address, attestation.CodeSize, attestation.CodeHash)
		state, err := client.AttestDeploymentState(ctx, id, contracts.RegistryVersion)
		if err != nil {
			t.Fatalf("%s state attestation: %v", id, err)
		}
		if err := state.ValidateCanonicalResources(); err != nil {
			t.Fatalf("%s canonical resource attestation: %v", id, err)
		}
		t.Logf("%s owner=%s fee_recipient=%s fee_bps=%d paused=%v usdc=%s eurc=%s router=%s permit2=%s pool_manager=%s pool_fee=%d tick_spacing=%d", id, state.Owner, state.FeeRecipient, state.FeeBPS, state.Paused, state.USDC, state.EURC, state.UniversalRouter, state.Permit2, state.PoolManager, state.PoolFee, state.PoolTickSpacing)
	}
	t.Logf("Arc Mainnet OK chain=%s head=%d", chainID, blockNumber)
}
