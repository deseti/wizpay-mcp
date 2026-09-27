package contracts_test

import (
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

func TestCanonicalArcMainnetResources(t *testing.T) {
	resources := contracts.DefaultArcMainnetResources()
	if err := resources.Validate(); err != nil {
		t.Fatal(err)
	}
	usdc, err := contracts.CanonicalToken("USDC")
	if err != nil {
		t.Fatal(err)
	}
	eurc, err := contracts.CanonicalToken("EURC")
	if err != nil {
		t.Fatal(err)
	}
	if usdc.Address != contracts.AddressUSDCMainnet || usdc.Decimals != 6 || usdc.ChainID != "5042" || usdc.Network != "MAINNET" {
		t.Fatalf("USDC = %#v", usdc)
	}
	if !contracts.AddressesEqual(eurc.Address, contracts.AddressEURCMainnet) || eurc.Decimals != 6 {
		t.Fatalf("EURC = %#v", eurc)
	}
	if resources.PoolManager != contracts.AddressUniswapV4PoolManager || resources.UniversalRouter != contracts.AddressUniswapUniversalRouter || resources.Permit2 != contracts.AddressPermit2 || resources.V4Quoter != contracts.AddressUniswapV4Quoter || resources.CandidatePoolID != contracts.PoolIDUSDCEURCCandidate || resources.PoolFee != 500 || resources.TickSpacing != 10 || resources.Hooks != contracts.AddressZero {
		t.Fatalf("resources = %#v", resources)
	}
}

func TestArcMainnetResourcesFailClosed(t *testing.T) {
	resources := contracts.DefaultArcMainnetResources()
	resources.Tokens[0].Decimals = 18
	if err := resources.Validate(); err == nil {
		t.Fatal("mismatched token decimals must be rejected")
	}
	resources = contracts.DefaultArcMainnetResources()
	resources.UniversalRouter = "0x1111111111111111111111111111111111111111"
	if err := resources.Validate(); err == nil {
		t.Fatal("mismatched router must be rejected")
	}
	if _, err := contracts.CanonicalToken("USDT"); err == nil {
		t.Fatal("unsupported token must be rejected")
	}
}
