package intents

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

func trackECrossPayrollParams(inputUSDC bool) Params {
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	in, out := Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: contracts.AddressUSDCMainnet, Symbol: "USDC", Decimals: 6}, Token{ChainID: contracts.ChainIDArcMainnet, Standard: "ERC20", Address: contracts.AddressEURCMainnet, Symbol: "EURC", Decimals: 6}
	if !inputUSDC {
		in, out = out, in
	}
	obligation := Amount{Decimal: "5", BaseUnits: "5000000", Decimals: 6}
	return Params{IntentID: "cross-payroll", Version: 1, ClientRequestID: "request", Nonce: "nonce", Type: TypePayroll,
		Ownership: Ownership{UserID: "user", IdentityProvider: "circle", ProviderUserReference: "provider-user", WalletBindingID: "binding", WalletBindingVersion: 1, WalletID: "wallet", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet},
		Financial: FinancialParameters{Payroll: &PayrollParameters{SchemaVersion: FinancialSchemaPhase12, Variant: PayrollVariantBatchSingleTokenOut, TokenIn: in, Recipients: []Recipient{{Address: "0x3333333333333333333333333333333333333333", TokenOut: out, MinAmountOut: obligation}, {Address: "0x4444444444444444444444444444444444444444", TokenOut: out, MinAmountOut: obligation}}, Total: Amount{Decimal: "10", BaseUnits: "10000000", Decimals: 6}, ReferenceID: "cross-reference", CrossToken: &CrossTokenPayrollParameters{GrossInput: Amount{Decimal: "11", BaseUnits: "11000000", Decimals: 6}, MinTotalOut: Amount{Decimal: "10", BaseUnits: "10000000", Decimals: 6}, MinHopPriceX36: "1", Deadline: now.Add(10 * time.Minute)}}},
		Route:     Route{Type: RouteAllowlistedContract, Reference: RouteReferencePayroll, Version: RouteVersionPayroll}, Constraints: Constraints{Deadline: now.Add(20 * time.Minute), PolicyReference: "policy:1"}, CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute)}
}

func TestTrackECrossTokenIntentDirectionsAndSafety(t *testing.T) {
	for _, usdc := range []bool{true, false} {
		if _, err := NewDraft(trackECrossPayrollParams(usdc)); err != nil {
			t.Fatalf("valid direction %v: %v", usdc, err)
		}
	}
	cases := []struct {
		name   string
		mutate func(*Params)
	}{
		{"same_token", func(p *Params) {
			for i := range p.Financial.Payroll.Recipients {
				p.Financial.Payroll.Recipients[i].TokenOut = p.Financial.Payroll.TokenIn
			}
		}},
		{"arbitrary_token", func(p *Params) { p.Financial.Payroll.TokenIn.Address = "0x9999999999999999999999999999999999999999" }},
		{"wrong_chain", func(p *Params) {
			p.Financial.Payroll.TokenIn.ChainID = "5042002"
			p.Ownership.ChainID = "5042002"
			for i := range p.Financial.Payroll.Recipients {
				p.Financial.Payroll.Recipients[i].TokenOut.ChainID = "5042002"
			}
		}},
		{"wrong_decimals", func(p *Params) {
			p.Financial.Payroll.TokenIn.Decimals = 18
			p.Financial.Payroll.CrossToken.GrossInput.Decimals = 18
		}},
		{"mixed_output", func(p *Params) { p.Financial.Payroll.Recipients[1].TokenOut = p.Financial.Payroll.TokenIn }},
		{"wrong_network", func(p *Params) { p.Ownership.Network = "TESTNET" }},
		{"zero_recipient", func(p *Params) { p.Financial.Payroll.Recipients[0].Address = contracts.AddressZero }},
		{"employer_recipient", func(p *Params) { p.Financial.Payroll.Recipients[0].Address = p.Ownership.WalletAddress }},
		{"contract_recipient", func(p *Params) { p.Financial.Payroll.Recipients[0].Address = contracts.AddressWizPayPayroll }},
		{"zero_obligation", func(p *Params) {
			p.Financial.Payroll.Recipients[0].MinAmountOut = Amount{Decimal: "0", BaseUnits: "0", Decimals: 6}
		}},
		{"too_many", func(p *Params) {
			line := p.Financial.Payroll.Recipients[0]
			p.Financial.Payroll.Recipients = make([]Recipient, 51)
			for i := range p.Financial.Payroll.Recipients {
				p.Financial.Payroll.Recipients[i] = line
				p.Financial.Payroll.Recipients[i].Address = "0x" + strings.Repeat("0", 38) + big.NewInt(int64(i+1)).Text(16)
			}
		}},
		{"long_reference", func(p *Params) { p.Financial.Payroll.ReferenceID = strings.Repeat("é", 33) }},
		{"zero_gross", func(p *Params) {
			p.Financial.Payroll.CrossToken.GrossInput = Amount{Decimal: "0", BaseUnits: "0", Decimals: 6}
		}},
		{"gross_over_uint128", func(p *Params) {
			p.Financial.Payroll.CrossToken.GrossInput = trackEAmountFromBase(new(big.Int).Lsh(big.NewInt(1), 128))
		}},
		{"zero_minimum", func(p *Params) {
			p.Financial.Payroll.CrossToken.MinTotalOut = Amount{Decimal: "0", BaseUnits: "0", Decimals: 6}
		}},
		{"minimum_over_uint128", func(p *Params) {
			p.Financial.Payroll.CrossToken.MinTotalOut = trackEAmountFromBase(new(big.Int).Lsh(big.NewInt(1), 128))
		}},
		{"minimum_below_obligations", func(p *Params) {
			p.Financial.Payroll.CrossToken.MinTotalOut = Amount{Decimal: "9", BaseUnits: "9000000", Decimals: 6}
		}},
		{"zero_price", func(p *Params) { p.Financial.Payroll.CrossToken.MinHopPriceX36 = "0" }},
		{"expired_deadline", func(p *Params) { p.Financial.Payroll.CrossToken.Deadline = p.CreatedAt }},
		{"deadline_over_window", func(p *Params) {
			p.Financial.Payroll.CrossToken.Deadline = p.CreatedAt.Add(20*time.Minute + time.Second)
			p.Constraints.Deadline = p.CreatedAt.Add(21 * time.Minute)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := trackECrossPayrollParams(true)
			tc.mutate(&p)
			if _, err := NewDraft(p); err == nil {
				t.Fatal("unsafe cross-token payroll accepted")
			}
		})
	}
}

func trackEAmountFromBase(value *big.Int) Amount {
	digits := value.String()
	if len(digits) <= 6 {
		digits = strings.Repeat("0", 7-len(digits)) + digits
	}
	whole, fraction := digits[:len(digits)-6], strings.TrimRight(digits[len(digits)-6:], "0")
	decimal := whole
	if fraction != "" {
		decimal += "." + fraction
	}
	return Amount{Decimal: decimal, BaseUnits: value.String(), Decimals: 6}
}

func TestTrackECrossTokenDigestBindsAggregateMaterial(t *testing.T) {
	base, _ := NewDraft(trackECrossPayrollParams(true))
	base, _ = base.Transition(StatusCreated, trackECrossPayrollParams(true).CreatedAt.Add(time.Second))
	for _, mutate := range []func(*Params){func(p *Params) {
		p.Financial.Payroll.CrossToken.GrossInput = Amount{Decimal: "12", BaseUnits: "12000000", Decimals: 6}
	}, func(p *Params) {
		p.Financial.Payroll.CrossToken.MinTotalOut = Amount{Decimal: "10.1", BaseUnits: "10100000", Decimals: 6}
	}, func(p *Params) { p.Financial.Payroll.CrossToken.MinHopPriceX36 = "2" }, func(p *Params) {
		p.Financial.Payroll.CrossToken.Deadline = p.Financial.Payroll.CrossToken.Deadline.Add(time.Second)
	}, func(p *Params) { p.Financial.Payroll.ReferenceID = "other-reference" }} {
		p := trackECrossPayrollParams(true)
		mutate(&p)
		value, err := NewDraft(p)
		if err != nil {
			t.Fatal(err)
		}
		value, _ = value.Transition(StatusCreated, p.CreatedAt.Add(time.Second))
		if value.Digest() == base.Digest() {
			t.Fatal("digest did not bind cross-token material")
		}
	}
}
