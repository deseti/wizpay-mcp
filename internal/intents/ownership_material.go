package intents

// canonicalOwnership keeps the pre-WP1 bytes when WalletProvider is absent.
// The pointer exists only in freshly constructed canonical material, never domain state.
type canonicalOwnership struct {
	UserID                string  `json:"user_id"`
	IdentityProvider      string  `json:"identity_provider"`
	WalletProvider        *string `json:"wallet_provider,omitempty"`
	ProviderUserReference string  `json:"provider_user_reference"`
	WalletBindingID       string  `json:"wallet_binding_id"`
	WalletBindingVersion  uint64  `json:"wallet_binding_version"`
	WalletID              string  `json:"wallet_id"`
	WalletAddress         string  `json:"wallet_address"`
	ChainID               string  `json:"chain_id"`
	Network               string  `json:"network"`
}

func ownershipMaterial(o Ownership) canonicalOwnership {
	var provider *string
	if o.WalletProvider != "" {
		value := o.WalletProvider
		provider = &value
	}
	return canonicalOwnership{o.UserID, o.IdentityProvider, provider, o.ProviderUserReference, o.WalletBindingID, o.WalletBindingVersion, o.WalletID, o.WalletAddress, o.ChainID, o.Network}
}
