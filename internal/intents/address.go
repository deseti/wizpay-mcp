package intents

import (
	"fmt"
	"unicode/utf8"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

// normalizeEVMAddress returns the lowercase 0x-prefixed form of a valid address.
// Invalid input is returned trimmed for fail-closed comparison.
func normalizeEVMAddress(value string) string {
	return contracts.NormalizeAddress(value)
}

func validateEVMAddress(name, value string) error {
	if !contracts.ValidAddress(value) {
		return fmt.Errorf("%s is not a valid EVM address", name)
	}
	return nil
}

func validateNonZeroEVMAddress(name, value string) error {
	if err := validateEVMAddress(name, value); err != nil {
		return err
	}
	if contracts.NormalizeAddress(value) == "0x0000000000000000000000000000000000000000" {
		return fmt.Errorf("%s must not be the zero address", name)
	}
	return nil
}

func addressesEqual(a, b string) bool {
	return contracts.AddressesEqual(a, b)
}

// normalizePayrollAddresses lowercases Phase 12 EVM address fields in place.
func normalizePayrollAddresses(p *PayrollParameters) {
	if p == nil {
		return
	}
	if !p.TokenIn.IsZero() && contracts.ValidAddress(p.TokenIn.Address) {
		p.TokenIn.Address = normalizeEVMAddress(p.TokenIn.Address)
	}
	if !p.Token.IsZero() && contracts.ValidAddress(p.Token.Address) {
		p.Token.Address = normalizeEVMAddress(p.Token.Address)
	}
	for i := range p.Recipients {
		if contracts.ValidAddress(p.Recipients[i].Address) {
			p.Recipients[i].Address = normalizeEVMAddress(p.Recipients[i].Address)
		}
		if !p.Recipients[i].TokenOut.IsZero() && contracts.ValidAddress(p.Recipients[i].TokenOut.Address) {
			p.Recipients[i].TokenOut.Address = normalizeEVMAddress(p.Recipients[i].TokenOut.Address)
		}
	}
}

// normalizeSwapAddresses lowercases Phase 12 EVM address fields in place.
func normalizeSwapAddresses(p *SwapParameters) {
	if p == nil {
		return
	}
	if contracts.ValidAddress(p.InputToken.Address) {
		p.InputToken.Address = normalizeEVMAddress(p.InputToken.Address)
	}
	if contracts.ValidAddress(p.OutputToken.Address) {
		p.OutputToken.Address = normalizeEVMAddress(p.OutputToken.Address)
	}
	if p.Router != "" && contracts.ValidAddress(p.Router) {
		p.Router = normalizeEVMAddress(p.Router)
	}
	if p.Recipient != "" && contracts.ValidAddress(p.Recipient) {
		p.Recipient = normalizeEVMAddress(p.Recipient)
	}
	if p.Quote != nil && p.Quote.Router != "" && contracts.ValidAddress(p.Quote.Router) {
		p.Quote.Router = normalizeEVMAddress(p.Quote.Router)
	}
}

// normalizeOwnershipWalletAddress lowercases a valid ownership wallet address.
func normalizeOwnershipWalletAddress(o *Ownership) {
	if o == nil {
		return
	}
	if contracts.ValidAddress(o.WalletAddress) {
		o.WalletAddress = normalizeEVMAddress(o.WalletAddress)
	}
}

func validateBoundedReferenceID(name, value string, maxRunes int) error {
	if err := validateText(name, value); err != nil {
		return err
	}
	if utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("%s exceeds %d characters", name, maxRunes)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s contains control characters", name)
		}
	}
	return nil
}
