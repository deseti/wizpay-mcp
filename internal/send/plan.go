// Package send implements the isolated SEND lifecycle for one direct canonical
// ERC-20 transfer on Arc Mainnet.
package send

import (
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

// Plan is immutable SEND material derived from one frozen intent.
type Plan struct {
	intentID     string
	intentDigest string
	token        intents.Token
	recipient    string
	amount       intents.Amount
	wallet       string
	provider     providers.Plan
}

func (p Plan) IntentID() string             { return p.intentID }
func (p Plan) IntentDigest() string         { return p.intentDigest }
func (p Plan) Token() intents.Token         { return p.token }
func (p Plan) Recipient() string            { return p.recipient }
func (p Plan) Amount() intents.Amount       { return p.amount }
func (p Plan) WalletAddress() string        { return p.wallet }
func (p Plan) ProviderPlan() providers.Plan { return p.provider }
