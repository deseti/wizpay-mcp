package storage

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/wallet"
)

// CreateBindingResult distinguishes an exact idempotent replay from a newly
// created binding without allowing duplicate wallet ownership.
type CreateBindingResult struct {
	Binding wallet.Binding
	Created bool
}

// WalletBindingRepository stores binding metadata only. CreateBinding must use
// the validated wallet.Binding.DuplicateKey deterministically: an identical
// replay returns the existing binding with Created=false; conflicting
// ownership fails closed.
type WalletBindingRepository interface {
	FindBindingByID(context.Context, Scope, string) (wallet.Binding, error)
	FindBindingByWallet(ctx context.Context, scope Scope, chainID, network, address string) (wallet.Binding, error)
	CreateBinding(context.Context, Scope, wallet.Binding) (CreateBindingResult, error)
	UpdateBinding(ctx context.Context, scope Scope, binding wallet.Binding, expectedVersion uint64) (wallet.Binding, error)
}
