package postgres

import (
	"context"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/wallet"
	"testing"
	"time"
)

func TestWP1SeparatedProviderPersistenceAndLegacyIntegrity(t *testing.T) {
	ctx := context.Background()
	f := createBaseFixture(t, false)
	before := f.intent.Digest()
	loaded, err := integrationStore.FindIntentByID(ctx, f.scope, f.intent.IntentID())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest() != before || loaded.Ownership().WalletProvider != "" {
		t.Fatal("legacy intent changed")
	}
	b, err := wallet.NewBinding(wallet.BindingParams{BindingID: unique("external-binding"), Version: 1, UserID: f.scope.ActorID(), Provider: "EXTERNAL_EVM", ProviderUserReference: unique("external-ref"), WalletID: unique("external-wallet"), Address: unique("external-address"), ChainID: f.binding.ChainID(), Network: f.binding.Network(), Status: wallet.BindingStatusActive, VerificationReference: "local-fixture-only", CreatedAt: fixtureNow.Add(-time.Hour), VerifiedAt: fixtureNow.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.CreateBinding(ctx, f.scope, b); err != nil {
		t.Fatal(err)
	}
	o := intents.Ownership{UserID: f.scope.ActorID(), IdentityProvider: f.identity.Provider(), WalletProvider: b.Provider(), ProviderUserReference: b.ProviderUserReference(), WalletBindingID: b.BindingID(), WalletBindingVersion: b.Version(), WalletID: b.WalletID(), WalletAddress: b.Address(), ChainID: b.ChainID(), Network: b.Network()}
	params := intents.Params{IntentID: unique("separated-intent"), Version: 1, ClientRequestID: unique("request"), Nonce: unique("nonce"), Type: f.intent.Type(), Ownership: o, Financial: f.intent.Financial(), Route: f.intent.Route(), Constraints: f.intent.Constraints(), CreatedAt: f.intent.CreatedAt(), ExpiresAt: f.intent.ExpiresAt()}
	i, err := intents.NewDraft(params)
	if err != nil {
		t.Fatal(err)
	}
	i, err = i.Transition(intents.StatusCreated, fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.CreateIntent(ctx, f.scope, i); err != nil {
		t.Fatal(err)
	}
	loaded, err = integrationStore.FindIntentByID(ctx, f.scope, i.IntentID())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest() != i.Digest() || loaded.Ownership() != o {
		t.Fatal("separated ownership did not round trip")
	}
	other, err := storage.NewScope(unique("other-tenant"), f.scope.ActorID(), unique("request"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.FindBindingByID(ctx, other, b.BindingID()); err == nil {
		t.Fatal("cross-tenant read accepted")
	}
	if _, err = integrationPool.Exec(ctx, `UPDATE intents SET wallet_provider='other',lifecycle_version=lifecycle_version+1 WHERE tenant_id=$1 AND intent_id=$2`, f.scope.TenantID(), i.IntentID()); err == nil {
		t.Fatal("frozen provider changed")
	}
	// Even direct inserts cannot substitute the provider or binding version.
	for _, kind := range []string{"provider", "version", "owner", "tenant"} {
		t.Run(kind, func(t *testing.T) {
			p := params
			p.IntentID = unique("invalid-intent")
			p.ClientRequestID = unique("invalid-request")
			p.Nonce = unique("invalid-nonce")
			scope := f.scope
			switch kind {
			case "provider":
				p.Ownership.WalletProvider = "other"
			case "version":
				p.Ownership.WalletBindingVersion++
			case "owner":
				p.Ownership.UserID = "other-user"
			case "tenant":
				scope = other
			}
			value, err := intents.NewDraft(p)
			if err != nil {
				t.Fatal(err)
			}
			arg, err := intentCreateParams(scope, value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = integrationStore.queries.CreateIntent(ctx, arg); err == nil {
				t.Fatal("database accepted invalid wallet relationship")
			}
		})
	}
	loaded, err = integrationStore.FindIntentByID(ctx, f.scope, f.intent.IntentID())
	if err != nil || loaded.Digest() != before {
		t.Fatal("legacy integrity lost")
	}
}
