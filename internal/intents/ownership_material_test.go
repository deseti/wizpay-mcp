package intents

import (
	"bytes"
	"testing"
	"time"
)

// This is the exact pre-WP1 ownership representation, deliberately independent
// of the production type so newly added fields cannot silently change this proof.
type legacyOwnership struct {
	UserID                string `json:"user_id"`
	IdentityProvider      string `json:"identity_provider"`
	ProviderUserReference string `json:"provider_user_reference"`
	WalletBindingID       string `json:"wallet_binding_id"`
	WalletBindingVersion  uint64 `json:"wallet_binding_version"`
	WalletID              string `json:"wallet_id"`
	WalletAddress         string `json:"wallet_address"`
	ChainID               string `json:"chain_id"`
	Network               string `json:"network"`
}

func TestLegacyOwnershipCanonicalBytesAndDigestRemainExact(t *testing.T) {
	p := testParams()
	o := p.Ownership
	legacy := struct {
		IntentID        string              `json:"intent_id"`
		Version         uint64              `json:"intent_version"`
		ClientRequestID string              `json:"client_request_id"`
		Nonce           string              `json:"nonce"`
		Type            Type                `json:"intent_type"`
		Ownership       legacyOwnership     `json:"ownership"`
		Financial       FinancialParameters `json:"financial_parameters"`
		Route           Route               `json:"route"`
		Constraints     Constraints         `json:"constraints"`
		CreatedAt       time.Time           `json:"created_at"`
		ExpiresAt       time.Time           `json:"expires_at"`
	}{p.IntentID, p.Version, p.ClientRequestID, p.Nonce, p.Type, legacyOwnership{o.UserID, o.IdentityProvider, o.ProviderUserReference, o.WalletBindingID, o.WalletBindingVersion, o.WalletID, o.WalletAddress, o.ChainID, o.Network}, p.Financial, p.Route, p.Constraints, p.CreatedAt, p.ExpiresAt}
	before, err := canonicalJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}
	after, err := canonicalMaterial(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("legacy canonical bytes changed")
	}
	frozen := mustTransition(t, mustDraft(t, p), StatusCreated, testNow)
	if frozen.Digest() != digestBytes(before) {
		t.Fatal("legacy digest changed")
	}
	if _, err = Restore(p, frozen.Status(), frozen.Digest(), frozen.LifecycleRevision()); err != nil {
		t.Fatal(err)
	}
	p.Ownership.WalletProvider = "EXTERNAL_EVM"
	if _, err = Restore(p, frozen.Status(), frozen.Digest(), frozen.LifecycleRevision()); err == nil {
		t.Fatal("historical snapshot silently upgraded")
	}
	explicit := mustTransition(t, mustDraft(t, p), StatusCreated, testNow)
	if explicit.Digest() == frozen.Digest() {
		t.Fatal("explicit wallet provider is not digest bound")
	}
	p.Ownership.WalletProvider = "other"
	if _, err = Restore(p, explicit.Status(), explicit.Digest(), explicit.LifecycleRevision()); err == nil {
		t.Fatal("provider substitution accepted")
	}
}
