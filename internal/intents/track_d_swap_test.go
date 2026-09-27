package intents

import (
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

func TestTrackDSwapIntentCanonicalDirectionsAndSafety(t *testing.T) {
	validEURC := phase12SwapParams()
	validEURC.Financial.Swap.InputToken, validEURC.Financial.Swap.OutputToken = validEURC.Financial.Swap.OutputToken, validEURC.Financial.Swap.InputToken
	if _, err := NewDraft(validEURC); err != nil {
		t.Fatalf("EURC to USDC: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*Params)
	}{
		{"same_token", func(p *Params) { p.Financial.Swap.OutputToken = p.Financial.Swap.InputToken }},
		{"arbitrary_token", func(p *Params) { p.Financial.Swap.InputToken.Address = "0x9999999999999999999999999999999999999999" }},
		{"wrong_chain", func(p *Params) {
			p.Financial.Swap.InputToken.ChainID = "5042002"
			p.Financial.Swap.OutputToken.ChainID = "5042002"
			p.Ownership.ChainID = "5042002"
		}},
		{"wrong_network", func(p *Params) { p.Ownership.Network = "TESTNET" }},
		{"wrong_decimals", func(p *Params) { p.Financial.Swap.InputToken.Decimals = 18; p.Financial.Swap.InputAmount.Decimals = 18 }},
		{"zero_min_hop", func(p *Params) { p.Financial.Swap.MinHopPriceX36 = "0" }},
		{"deadline_over_20m", func(p *Params) {
			p.Financial.Swap.Deadline = p.CreatedAt.Add(20*time.Minute + time.Second)
			p.Financial.Swap.Quote.ExpiresAt = p.CreatedAt.Add(21 * time.Minute)
			p.Constraints.Deadline = p.CreatedAt.Add(22 * time.Minute)
		}},
		{"deadline_after_quote", func(p *Params) { p.Financial.Swap.Quote.ExpiresAt = p.CreatedAt.Add(9 * time.Minute) }},
		{"arbitrary_router", func(p *Params) {
			p.Financial.Swap.Router = contracts.AddressWizPayPayroll
			p.Financial.Swap.Quote.Router = contracts.AddressWizPayPayroll
		}},
		{"arbitrary_recipient", func(p *Params) { p.Financial.Swap.Recipient = "0x9999999999999999999999999999999999999999" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := phase12SwapParams()
			q := *p.Financial.Swap.Quote
			p.Financial.Swap.Quote = &q
			tc.mutate(&p)
			if _, err := NewDraft(p); err == nil {
				t.Fatal("unsafe swap accepted")
			}
		})
	}
}
