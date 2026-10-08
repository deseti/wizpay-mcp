package send

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/wallet"
	"github.com/ethereum/go-ethereum/core/types"
)

// ValidateSignedTransaction independently checks an EOA envelope against one
// approved frozen SEND intent and its current verified binding. It performs no
// I/O, authorization, submission, or receipt verification. Success here is not
// financial success and does not enable the disabled Circle execution path.
func ValidateSignedTransaction(intent intents.Intent, binding wallet.Binding, raw []byte) error {
	plan, err := NewPlanner().Plan(intent)
	if err != nil {
		return err
	}
	owner := intent.Ownership()
	if err := binding.EnsureAuthorizable(owner.UserID); err != nil {
		return err
	}
	if binding.BindingID() != owner.WalletBindingID || binding.Version() != owner.WalletBindingVersion ||
		binding.Provider() != owner.EffectiveWalletProvider() || binding.ProviderUserReference() != owner.ProviderUserReference ||
		binding.WalletID() != owner.WalletID || binding.Address() != owner.WalletAddress ||
		binding.ChainID() != owner.ChainID || binding.Network() != owner.Network {
		return fmt.Errorf("signed SEND wallet binding mismatch")
	}
	if len(raw) == 0 || len(raw) > 4096 {
		return fmt.Errorf("signed SEND envelope size is invalid")
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return fmt.Errorf("signed SEND envelope is malformed")
	}
	// Deliberately exclude blob, authorization-list, and future transaction
	// types; they introduce semantics beyond a single direct ERC-20 SEND.
	if tx.Type() != types.LegacyTxType && tx.Type() != types.AccessListTxType && tx.Type() != types.DynamicFeeTxType {
		return fmt.Errorf("signed SEND envelope type is unsupported")
	}
	canonical, err := tx.MarshalBinary()
	if err != nil || !bytes.Equal(canonical, raw) {
		return fmt.Errorf("signed SEND envelope is noncanonical")
	}
	if !tx.Protected() || tx.ChainId().Cmp(big.NewInt(5042)) != 0 {
		return fmt.Errorf("signed SEND chain must be 5042")
	}
	if tx.To() == nil || !contracts.AddressesEqual(tx.To().Hex(), plan.Token().Address) {
		return fmt.Errorf("signed SEND token target mismatch")
	}
	if tx.Value().Sign() != 0 {
		return fmt.Errorf("signed SEND value must be zero")
	}
	recipient, amount, err := decodeTransfer(tx.Data())
	if err != nil || !contracts.AddressesEqual(recipient, plan.Recipient()) || amount.String() != plan.Amount().BaseUnits {
		return fmt.Errorf("signed SEND calldata mismatch")
	}
	sender, err := types.Sender(types.LatestSignerForChainID(big.NewInt(5042)), &tx)
	if err != nil || !contracts.AddressesEqual(sender.Hex(), binding.Address()) {
		return fmt.Errorf("signed SEND signer mismatch")
	}
	return nil
}
