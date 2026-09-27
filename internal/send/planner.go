package send

import (
	"fmt"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

type Planner struct{}

func NewPlanner() Planner { return Planner{} }

// Plan accepts only a frozen, execution-authorized SEND intent. There are no
// caller-controlled replacement financial arguments.
func (Planner) Plan(intent intents.Intent) (Plan, error) {
	if err := intent.Validate(); err != nil {
		return Plan{}, fmt.Errorf("send intent is invalid: %w", err)
	}
	if intent.Type() != intents.TypeSend {
		return Plan{}, fmt.Errorf("send planner requires SEND intent")
	}
	if intent.Digest() == "" || (intent.Status() != intents.StatusApproved && intent.Status() != intents.StatusReadyForExecution) {
		return Plan{}, fmt.Errorf("send planner requires an approved frozen intent")
	}
	financial := intent.Financial().Send
	owner := intent.Ownership()
	canonical, err := contracts.CanonicalToken(financial.Token.Symbol)
	if err != nil || financial.Token.ChainID != contracts.ChainIDArcMainnet || owner.ChainID != contracts.ChainIDArcMainnet ||
		owner.Network != contracts.NetworkArcMainnet || !contracts.AddressesEqual(financial.Token.Address, canonical.Address) ||
		financial.Token.Decimals != canonical.Decimals || financial.Token.Standard != "ERC20" {
		return Plan{}, fmt.Errorf("send requires a canonical Arc Mainnet token")
	}
	providerPlan, err := providers.NewTokenTransferPlan(providers.TokenTransferParams{
		WalletBindingID: owner.WalletBindingID, WalletID: owner.WalletID, WalletAddress: owner.WalletAddress,
		ChainID: owner.ChainID, Network: owner.Network, Destination: financial.Recipient,
		TokenID: financial.Token.Symbol, TokenAddress: canonical.Address, TokenDecimals: canonical.Decimals,
		Amount: financial.Amount.Decimal, AmountBaseUnits: financial.Amount.BaseUnits,
	})
	if err != nil {
		return Plan{}, fmt.Errorf("build send provider plan: %w", err)
	}
	return Plan{intentID: intent.IntentID(), intentDigest: intent.Digest(), token: financial.Token,
		recipient: financial.Recipient, amount: financial.Amount, wallet: owner.WalletAddress, provider: providerPlan}, nil
}
