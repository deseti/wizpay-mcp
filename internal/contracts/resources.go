package contracts

import "fmt"

const (
	AddressUSDCMainnet = "0x3600000000000000000000000000000000000000"
	AddressEURCMainnet = "0xbEf5f6d51CB62b58e6A8f77868681825C6fe21c1"

	AddressUniswapV4PoolManager          = "0x8366a39CC670B4001A1121B8F6A443A643e40951"
	AddressUniswapUniversalRouter        = "0x4fca4a51ab4f23a7447b3284fbd7d73289a89fb1"
	AddressPermit2                       = "0x000000000022D473030F116dDEE9F6B43aC78BA3"
	AddressUniswapV4Quoter               = "0x8dc178efb8111bb0973dd9d722ebeff267c98f94"
	AddressZero                          = "0x0000000000000000000000000000000000000000"
	PoolIDUSDCEURCCandidate              = "0xeb0fd02fb8044d5514fb6e165ee134fd547eff0378bb33b76f4b81d8b03bd1ae"
	UniswapV4USDCEURCFee          uint32 = 500
	UniswapV4TickSpacing          int32  = 10
	CanonicalTokenDecimals        uint8  = 6
)

// TokenResource is a closed Arc Mainnet token identity. It is registry
// metadata only and grants no execution authority.
type TokenResource struct {
	Symbol   string `json:"symbol"`
	ChainID  string `json:"chain_id"`
	Network  string `json:"network"`
	Address  string `json:"address"`
	Decimals uint8  `json:"decimals"`
}

// ArcMainnetResources is the reviewed Track A resource set. The candidate pool
// identity does not assert liquidity, quote quality, or executable availability.
type ArcMainnetResources struct {
	Tokens          []TokenResource `json:"tokens"`
	PoolManager     string          `json:"pool_manager"`
	UniversalRouter string          `json:"universal_router"`
	Permit2         string          `json:"permit2"`
	V4Quoter        string          `json:"v4_quoter"`
	CandidatePoolID string          `json:"candidate_pool_id"`
	PoolFee         uint32          `json:"pool_fee"`
	TickSpacing     int32           `json:"tick_spacing"`
	Hooks           string          `json:"hooks"`
}

// DefaultArcMainnetResources returns a defensive copy of the canonical,
// non-executable Arc Mainnet resource registry.
func DefaultArcMainnetResources() ArcMainnetResources {
	return ArcMainnetResources{
		Tokens: []TokenResource{
			{Symbol: "USDC", ChainID: ChainIDArcMainnet, Network: NetworkArcMainnet, Address: AddressUSDCMainnet, Decimals: CanonicalTokenDecimals},
			{Symbol: "EURC", ChainID: ChainIDArcMainnet, Network: NetworkArcMainnet, Address: AddressEURCMainnet, Decimals: CanonicalTokenDecimals},
		},
		PoolManager:     AddressUniswapV4PoolManager,
		UniversalRouter: AddressUniswapUniversalRouter,
		Permit2:         AddressPermit2,
		V4Quoter:        AddressUniswapV4Quoter,
		CandidatePoolID: PoolIDUSDCEURCCandidate,
		PoolFee:         UniswapV4USDCEURCFee,
		TickSpacing:     UniswapV4TickSpacing,
		Hooks:           AddressZero,
	}
}

// CanonicalToken resolves only USDC or EURC and returns no arbitrary token.
func CanonicalToken(symbol string) (TokenResource, error) {
	for _, token := range DefaultArcMainnetResources().Tokens {
		if token.Symbol == symbol {
			return token, nil
		}
	}
	return TokenResource{}, fmt.Errorf("token %q is not registered for Arc Mainnet", symbol)
}

// Validate fails closed unless every field exactly matches the reviewed
// Mainnet resource registry.
func (r ArcMainnetResources) Validate() error {
	want := DefaultArcMainnetResources()
	if len(r.Tokens) != len(want.Tokens) {
		return fmt.Errorf("Arc Mainnet token registry must contain exactly USDC and EURC")
	}
	for _, symbol := range []string{"USDC", "EURC"} {
		got, ok := tokenBySymbol(r.Tokens, symbol)
		if !ok {
			return fmt.Errorf("Arc Mainnet token registry is missing %s", symbol)
		}
		expected, _ := CanonicalToken(symbol)
		if got.ChainID != expected.ChainID || got.Network != expected.Network ||
			!AddressesEqual(got.Address, expected.Address) || got.Decimals != expected.Decimals {
			return fmt.Errorf("Arc Mainnet %s resource does not match the canonical identity", symbol)
		}
	}
	if !AddressesEqual(r.PoolManager, want.PoolManager) ||
		!AddressesEqual(r.UniversalRouter, want.UniversalRouter) ||
		!AddressesEqual(r.Permit2, want.Permit2) ||
		!AddressesEqual(r.V4Quoter, want.V4Quoter) ||
		r.CandidatePoolID != want.CandidatePoolID || r.PoolFee != want.PoolFee ||
		r.TickSpacing != want.TickSpacing || !AddressesEqual(r.Hooks, want.Hooks) {
		return fmt.Errorf("Arc Mainnet Uniswap V4 resources do not match the canonical registry")
	}
	return nil
}

func tokenBySymbol(tokens []TokenResource, symbol string) (TokenResource, bool) {
	for _, token := range tokens {
		if token.Symbol == symbol {
			return token, true
		}
	}
	return TokenResource{}, false
}
