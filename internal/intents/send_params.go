package intents

import (
	"fmt"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

const (
	RouteReferenceSend = "ARC_MAINNET_ERC20_TRANSFER"
	RouteVersionSend   = uint64(1)
)

// SendParameters freezes one direct canonical ERC-20 transfer. It contains no
// router, calldata, spender, sender override, or native-value material.
type SendParameters struct {
	Token     Token  `json:"token"`
	Recipient string `json:"recipient"`
	Amount    Amount `json:"amount"`
}

func (p SendParameters) validate() error {
	if err := p.Token.ValidatePhase12Token("send token"); err != nil {
		return err
	}
	canonical, err := contracts.CanonicalToken(p.Token.Symbol)
	if err != nil {
		return fmt.Errorf("send token is unsupported")
	}
	if p.Token.ChainID != canonical.ChainID || p.Token.Standard != "ERC20" ||
		!contracts.AddressesEqual(p.Token.Address, canonical.Address) || p.Token.Decimals != canonical.Decimals {
		return fmt.Errorf("send token does not match the canonical Arc Mainnet registry")
	}
	if err := validateNonZeroEVMAddress("send recipient", p.Recipient); err != nil {
		return err
	}
	if !p.Amount.IsPositive() {
		return fmt.Errorf("send amount must be positive")
	}
	if p.Amount.Decimals != canonical.Decimals {
		return fmt.Errorf("send amount decimals do not match canonical token")
	}
	return nil
}

func validateSendRoute(route Route) error {
	if route.Type != RouteDirectWallet || route.Reference != RouteReferenceSend || route.Version != RouteVersionSend {
		return fmt.Errorf("SEND requires the canonical direct ERC-20 route")
	}
	return nil
}
