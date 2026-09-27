package intents

import (
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

var sendTestNow = time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)

func sendParams(symbol string) Params {
	resource, _ := contracts.CanonicalToken(symbol)
	return Params{IntentID: "send_1", Version: 1, ClientRequestID: "client_1", Nonce: "nonce_1", Type: TypeSend,
		Ownership: Ownership{UserID: "user", IdentityProvider: "provider", ProviderUserReference: "provider-user", WalletBindingID: "binding", WalletBindingVersion: 1, WalletID: "wallet", WalletAddress: "0x1111111111111111111111111111111111111111", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet},
		Financial: FinancialParameters{Send: &SendParameters{Token: Token{ChainID: resource.ChainID, Standard: "ERC20", Address: resource.Address, Symbol: resource.Symbol, Decimals: resource.Decimals}, Recipient: "0x2222222222222222222222222222222222222222", Amount: Amount{Decimal: "1.25", BaseUnits: "1250000", Decimals: 6}}},
		Route:     Route{Type: RouteDirectWallet, Reference: RouteReferenceSend, Version: RouteVersionSend}, Constraints: Constraints{Deadline: sendTestNow.Add(time.Hour), PolicyReference: "policy:1"}, CreatedAt: sendTestNow, ExpiresAt: sendTestNow.Add(time.Hour)}
}

func freezeSend(t *testing.T, params Params) Intent {
	t.Helper()
	value, err := NewDraft(params)
	if err != nil {
		t.Fatal(err)
	}
	value, err = value.Transition(StatusCreated, sendTestNow)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestSendIntentAcceptsCanonicalUSDCAndEURC(t *testing.T) {
	for _, symbol := range []string{"USDC", "EURC"} {
		if _, err := NewDraft(sendParams(symbol)); err != nil {
			t.Fatalf("%s rejected: %v", symbol, err)
		}
	}
}

func TestSendIntentRejectsUnsafeMaterial(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Params)
	}{
		{"unsupported token", func(p *Params) { p.Financial.Send.Token.Symbol = "USDT" }},
		{"wrong chain", func(p *Params) { p.Ownership.ChainID = "5042002"; p.Financial.Send.Token.ChainID = "5042002" }},
		{"wrong decimals", func(p *Params) { p.Financial.Send.Token.Decimals = 18 }},
		{"zero amount", func(p *Params) { p.Financial.Send.Amount = Amount{Decimal: "0", BaseUnits: "0", Decimals: 6} }},
		{"zero recipient", func(p *Params) { p.Financial.Send.Recipient = contracts.AddressZero }},
		{"invalid recipient", func(p *Params) { p.Financial.Send.Recipient = "not-an-address" }},
		{"self send", func(p *Params) { p.Financial.Send.Recipient = p.Ownership.WalletAddress }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := sendParams("USDC")
			tt.mutate(&p)
			if _, err := NewDraft(p); err == nil {
				t.Fatal("unsafe SEND accepted")
			}
		})
	}
}

func TestSendDigestBindsFinancialMaterial(t *testing.T) {
	base := freezeSend(t, sendParams("USDC")).Digest()
	for _, mutate := range []func(*Params){
		func(p *Params) { p.Financial.Send.Recipient = "0x3333333333333333333333333333333333333333" },
		func(p *Params) {
			resource, _ := contracts.CanonicalToken("EURC")
			p.Financial.Send.Token.Address, p.Financial.Send.Token.Symbol = resource.Address, resource.Symbol
		},
		func(p *Params) { p.Financial.Send.Amount = Amount{Decimal: "2", BaseUnits: "2000000", Decimals: 6} },
	} {
		p := sendParams("USDC")
		mutate(&p)
		if got := freezeSend(t, p).Digest(); got == base {
			t.Fatal("digest did not bind changed SEND material")
		}
	}
}
