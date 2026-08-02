package wallet

import "context"

// Descriptor is provider-resolved public wallet metadata. It cannot contain
// key material, signing capability, credentials, or authorization tokens.
type Descriptor struct {
	WalletID string
	Address  string
	ChainID  string
	Network  string
}

// ResolveRequest identifies wallet metadata to resolve through a future
// provider adapter. ProviderUserReference is an opaque non-credential value.
type ResolveRequest struct {
	ProviderUserReference string
	WalletID              string
	ChainID               string
}

// VerificationResult is a metadata observation, not user authorization.
type VerificationResult struct {
	Verified              bool
	Descriptor            Descriptor
	VerificationReference string
}

// Provider defines future wallet metadata resolution and binding verification.
// Phase 2 intentionally provides no implementation.
type Provider interface {
	ResolveWallet(context.Context, ResolveRequest) (Descriptor, error)
	VerifyBinding(context.Context, Binding) (VerificationResult, error)
}
