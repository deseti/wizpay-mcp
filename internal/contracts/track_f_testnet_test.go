package contracts_test

import (
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractpayroll "github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	contractswap "github.com/deseti/wizpay-mcp/internal/contracts/swap"
)

func TestTrackFTestnetArtifactsCannotEnterMainnetExecutionSurface(t *testing.T) {
	registry := contracts.DefaultRegistry()
	for _, id := range []contracts.ContractID{contracts.ContractWizPayPayroll, contracts.ContractWizPaySwapExecutor} {
		if _, err := registry.Require(id, contracts.RegistryVersion, "5042002", "TESTNET"); err == nil {
			t.Fatalf("%s accepted Arc Testnet identity", id)
		}
	}
	for _, symbol := range []string{"USDC", "EURC"} {
		resource, err := contracts.CanonicalToken(symbol)
		if err != nil {
			t.Fatal(err)
		}
		if resource.ChainID != contracts.ChainIDArcMainnet || resource.Network != contracts.NetworkArcMainnet {
			t.Fatalf("%s resolved outside Arc Mainnet: %#v", symbol, resource)
		}
	}
	for _, obsolete := range []string{
		"routeAndPay(address,address,uint256,uint256,address,uint256)",
		"batchRouteAndPay(address,address[],uint256[],uint256,address,uint256)",
	} {
		if _, err := contractpayroll.MethodBySignature(obsolete); err == nil {
			t.Fatalf("obsolete Testnet Payroll selector accepted: %s", obsolete)
		}
	}
	for _, obsolete := range []string{
		"executeSwap(address,address,address,uint256,uint256,address,uint256)",
		"swap(address,address,uint256,uint256,address,uint256)",
	} {
		if _, err := contractswap.MethodBySignature(obsolete); err == nil {
			t.Fatalf("obsolete Testnet Swap selector accepted: %s", obsolete)
		}
	}
}
