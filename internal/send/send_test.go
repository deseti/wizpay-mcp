package send

import (
	"math/big"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type allowApproval struct{}

func (allowApproval) EnsureAuthorizes(intents.Intent, time.Time) error { return nil }

func approvedSend(t *testing.T, symbol string) intents.Intent {
	t.Helper()
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	resource, _ := contracts.CanonicalToken(symbol)
	p := intents.Params{IntentID: "send", Version: 1, ClientRequestID: "client", Nonce: "nonce", Type: intents.TypeSend,
		Ownership: intents.Ownership{UserID: "user", IdentityProvider: "provider", ProviderUserReference: "provider-user", WalletBindingID: "binding", WalletBindingVersion: 1, WalletID: "wallet", WalletAddress: "0x1111111111111111111111111111111111111111", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet},
		Financial: intents.FinancialParameters{Send: &intents.SendParameters{Token: intents.Token{ChainID: resource.ChainID, Standard: "ERC20", Address: resource.Address, Symbol: resource.Symbol, Decimals: resource.Decimals}, Recipient: "0x2222222222222222222222222222222222222222", Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}},
		Route:     intents.Route{Type: intents.RouteDirectWallet, Reference: intents.RouteReferenceSend, Version: intents.RouteVersionSend}, Constraints: intents.Constraints{Deadline: now.Add(time.Hour), PolicyReference: "policy:1"}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	v, err := intents.NewDraft(p)
	if err != nil {
		t.Fatal(err)
	}
	v, err = v.Transition(intents.StatusCreated, now)
	if err != nil {
		t.Fatal(err)
	}
	v, err = v.Transition(intents.StatusApprovalRequired, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	v, err = v.Approve(allowApproval{}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func receiptFor(plan Plan) providers.Receipt {
	amount, _ := new(big.Int).SetString(plan.Amount().BaseUnits, 10)
	input := make([]byte, 68)
	copy(input, transferSelector)
	copy(input[16:36], common.HexToAddress(plan.Recipient()).Bytes())
	amount.FillBytes(input[36:68])
	topic := func(address string) []byte {
		out := make([]byte, 32)
		copy(out[12:], common.HexToAddress(address).Bytes())
		return out
	}
	data := make([]byte, 32)
	amount.FillBytes(data)
	return providers.Receipt{Status: providers.ReceiptSuccess, ChainID: contracts.ChainIDArcMainnet, HasTransaction: true, From: plan.WalletAddress(), To: plan.Token().Address, Value: "0x0", Input: input, Logs: []providers.ReceiptLog{{Address: plan.Token().Address, Topics: [][]byte{crypto.Keccak256([]byte("Transfer(address,address,uint256)")), topic(plan.WalletAddress()), topic(plan.Recipient())}, Data: data}}}
}

func TestPlannerDerivesFrozenCanonicalTransfer(t *testing.T) {
	intent := approvedSend(t, "USDC")
	plan, err := NewPlanner().Plan(intent)
	if err != nil {
		t.Fatal(err)
	}
	p := plan.ProviderPlan()
	f := intent.Financial().Send
	if p.ChainID != "5042" || p.Network != "MAINNET" || p.WalletAddress != intent.Ownership().WalletAddress || p.TokenAddress != f.Token.Address || p.DestinationAddress != f.Recipient || p.AmountBaseUnits != f.Amount.BaseUnits {
		t.Fatalf("plan not bound to intent: %#v", p)
	}
}

func TestVerifierRequiresExactTransactionAndEvent(t *testing.T) {
	for _, symbol := range []string{"USDC", "EURC"} {
		t.Run(symbol, func(t *testing.T) {
			intent := approvedSend(t, symbol)
			plan, _ := NewPlanner().Plan(intent)
			if err := NewVerifier().Verify(intent, plan, receiptFor(plan)); err != nil {
				t.Fatal(err)
			}
		})
	}
	intent := approvedSend(t, "USDC")
	plan, _ := NewPlanner().Plan(intent)
	tests := []struct {
		name   string
		mutate func(*providers.Receipt)
	}{
		{"wrong chain", func(r *providers.Receipt) { r.ChainID = "1" }}, {"failed", func(r *providers.Receipt) { r.Status = providers.ReceiptReverted }},
		{"wrong sender", func(r *providers.Receipt) { r.From = "0x3333333333333333333333333333333333333333" }}, {"wrong token", func(r *providers.Receipt) { r.To = "0x3333333333333333333333333333333333333333" }},
		{"native value", func(r *providers.Receipt) { r.Value = "0x1" }}, {"wrong selector", func(r *providers.Receipt) { r.Input[0] = 0 }},
		{"malformed calldata", func(r *providers.Receipt) { r.Input = r.Input[:67] }},
		{"wrong recipient", func(r *providers.Receipt) { r.Input[35] = 0x33 }}, {"wrong amount", func(r *providers.Receipt) { r.Input[67]++ }},
		{"missing event", func(r *providers.Receipt) { r.Logs = nil }},
		{"wrong event emitter", func(r *providers.Receipt) { r.Logs[0].Address = "0x3333333333333333333333333333333333333333" }},
		{"wrong event from", func(r *providers.Receipt) { r.Logs[0].Topics[1][31]++ }},
		{"wrong event to", func(r *providers.Receipt) { r.Logs[0].Topics[2][31]++ }},
		{"wrong event amount", func(r *providers.Receipt) { r.Logs[0].Data[31]++ }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := receiptFor(plan)
			tt.mutate(&r)
			if err := NewVerifier().Verify(intent, plan, r); err == nil {
				t.Fatal("invalid evidence verified")
			}
		})
	}
}
