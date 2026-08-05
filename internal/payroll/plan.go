package payroll

import (
	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

// Plan is a deterministic, immutable Payroll contract-call plan. It does not
// represent approval, submit a transaction, or assert financial success.
type Plan struct {
	intentID        string
	intentDigest    string
	capability      intents.Type
	contractID      contracts.ContractID
	registryVersion uint
	chainID         string
	walletAddress   string
	encodedCall     contracts.EncodedCall
}

func (p Plan) IntentID() string                   { return p.intentID }
func (p Plan) IntentDigest() string               { return p.intentDigest }
func (p Plan) Capability() intents.Type           { return p.capability }
func (p Plan) ContractID() contracts.ContractID   { return p.contractID }
func (p Plan) RegistryVersion() uint              { return p.registryVersion }
func (p Plan) ChainID() string                    { return p.chainID }
func (p Plan) WalletAddress() string              { return p.walletAddress }
func (p Plan) EncodedCall() contracts.EncodedCall { return p.encodedCall.Clone() }
