package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/contracts"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

func TestCreatedSendWithSubMicrosecondTimestampsRoundTrips(t *testing.T) {
	ctx := context.Background()
	tenantID := unique("tenant")
	userID := unique("user")
	scope, err := storage.NewScope(tenantID, userID, unique("request"), unique("trace"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.CreateTenant(ctx, storage.Tenant{TenantID: tenantID, CreatedAt: fixtureNow}); err != nil {
		t.Fatal(err)
	}
	identity, err := auth.NewIdentity(userID, "test-provider", auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.CreateIdentity(ctx, scope, identity); err != nil {
		t.Fatal(err)
	}
	binding, err := wallet.NewBinding(wallet.BindingParams{
		BindingID: unique("binding"), Version: 1, UserID: userID,
		Provider: "test-provider", ProviderUserReference: unique("provider-user"), WalletID: unique("wallet"),
		Address: "0x1111111111111111111111111111111111111111", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet,
		Status: wallet.BindingStatusActive, VerificationReference: unique("verification"), CreatedAt: fixtureNow.Add(-2 * time.Hour), VerifiedAt: fixtureNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.CreateBinding(ctx, scope, binding); err != nil {
		t.Fatal(err)
	}

	createdAt := fixtureNow.Add(123456789 * time.Nanosecond)
	resource, _ := contracts.CanonicalToken("USDC")
	draft, err := intents.NewDraft(intents.Params{
		IntentID: unique("intent"), Version: 1, ClientRequestID: unique("client-request"), Nonce: unique("nonce"), Type: intents.TypeSend,
		Ownership: intents.Ownership{UserID: userID, IdentityProvider: binding.Provider(), ProviderUserReference: binding.ProviderUserReference(), WalletBindingID: binding.BindingID(), WalletBindingVersion: binding.Version(), WalletID: binding.WalletID(), WalletAddress: binding.Address(), ChainID: binding.ChainID(), Network: binding.Network()},
		Financial: intents.FinancialParameters{Send: &intents.SendParameters{Token: intents.Token{ChainID: resource.ChainID, Standard: "ERC20", Address: resource.Address, Symbol: resource.Symbol, Decimals: resource.Decimals}, Recipient: "0x2222222222222222222222222222222222222222", Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}},
		Route:     intents.Route{Type: intents.RouteDirectWallet, Reference: intents.RouteReferenceSend, Version: intents.RouteVersionSend}, Constraints: intents.Constraints{Deadline: createdAt.Add(30*time.Minute + 345678912*time.Nanosecond), PolicyReference: "policy:send"},
		CreatedAt: createdAt, ExpiresAt: createdAt.Add(time.Hour + 234567891*time.Nanosecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := draft.Transition(intents.StatusCreated, draft.CreatedAt())
	if err != nil {
		t.Fatal(err)
	}

	result, err := integrationStore.CreateIntent(ctx, scope, created)
	if err != nil {
		t.Fatalf("persist CREATED send: %v", err)
	}
	if !result.Created || result.Intent.Digest() != created.Digest() {
		t.Fatalf("persisted result = (created %t, digest %q), want (true, %q)", result.Created, result.Intent.Digest(), created.Digest())
	}
	if err := result.Intent.Validate(); err != nil {
		t.Fatalf("restored CREATED send is invalid: %v", err)
	}
	loaded, err := integrationStore.FindIntentByID(ctx, scope, created.IntentID())
	if err != nil {
		t.Fatalf("load CREATED send: %v", err)
	}
	if loaded.Digest() != created.Digest() || loaded.Status() != intents.StatusCreated {
		t.Fatalf("loaded status/digest = %s/%s, want %s/%s", loaded.Status(), loaded.Digest(), intents.StatusCreated, created.Digest())
	}
}

func TestIntentDraftFreezeRoundTripAndDatabaseInvariants(t *testing.T) {
	f := createBaseFixture(t, false)
	draft := newDraftSibling(t, f.intent)
	created, err := integrationStore.CreateIntent(context.Background(), f.scope, draft)
	if err != nil || !created.Created || created.Intent.Status() != intents.StatusDraft {
		t.Fatalf("persist draft = (%+v, %v)", created, err)
	}
	replay, err := integrationStore.CreateIntent(context.Background(), f.scope, draft)
	if err != nil || replay.Created || replay.Intent.LifecycleRevision() != draft.LifecycleRevision() {
		t.Fatalf("replay draft = (%+v, %v)", replay, err)
	}

	frozen, err := draft.Transition(intents.StatusCreated, fixtureNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	operation, err := intents.NewOperationIdentity(frozen)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := integrationStore.FreezeIntent(context.Background(), f.scope, frozen, draft.LifecycleRevision())
	if err != nil {
		t.Fatalf("freeze intent: %v", err)
	}
	loaded, err := integrationStore.FindIntentByID(context.Background(), f.scope, draft.IntentID())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status() != intents.StatusCreated || loaded.Digest() != frozen.Digest() || stored.Digest() != frozen.Digest() {
		t.Fatalf("frozen round trip status/digest = %s/%s", loaded.Status(), loaded.Digest())
	}
	if loaded.LifecycleRevision() != draft.LifecycleRevision()+1 {
		t.Fatalf("frozen lifecycle revision = %d, want %d", loaded.LifecycleRevision(), draft.LifecycleRevision()+1)
	}
	byOperation, err := integrationStore.FindIntentByOperationKey(context.Background(), f.scope, operation.OperationKey(), operation.Version())
	if err != nil || byOperation.IntentID() != draft.IntentID() {
		t.Fatalf("persisted operation identity = (%s, %v)", byOperation.IntentID(), err)
	}
	if _, err = integrationStore.FreezeIntent(context.Background(), f.scope, frozen, draft.LifecycleRevision()); apperrors.ToPublic(err).Code != apperrors.CodeExecutionConflict {
		t.Fatalf("stale freeze error = %v", err)
	}
	otherScope, err := storage.NewScope(f.scope.TenantID(), unique("other-user"), unique("request"), unique("trace"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.FreezeIntent(context.Background(), otherScope, frozen, draft.LifecycleRevision()); apperrors.ToPublic(err).Code != apperrors.CodeAuthorizationRequired {
		t.Fatalf("actor-scoped freeze error = %v", err)
	}

	other := newDraftSibling(t, f.intent)
	if _, err = integrationStore.CreateIntent(context.Background(), f.scope, other); err != nil {
		t.Fatal(err)
	}
	otherFrozen, err := other.Transition(intents.StatusCreated, fixtureNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	otherOperation, err := intents.NewOperationIdentity(otherFrozen)
	if err != nil {
		t.Fatal(err)
	}
	_, err = integrationPool.Exec(context.Background(), `UPDATE intents SET status='CREATED', nonce=$3, intent_digest=$4, operation_key=$5, operation_version=$6, lifecycle_version=lifecycle_version+1 WHERE tenant_id=$1 AND intent_id=$2`, f.scope.TenantID(), other.IntentID(), other.Nonce()+"-changed", otherFrozen.Digest(), otherOperation.OperationKey(), int64(otherOperation.Version()))
	requirePostgresConstraint(t, err)

	for _, test := range []struct {
		name      string
		statement string
		value     any
	}{
		{name: "digest_after_created", statement: `UPDATE intents SET intent_digest=$3, lifecycle_version=lifecycle_version+1 WHERE tenant_id=$1 AND intent_id=$2`, value: frozen.Digest() + "changed"},
		{name: "operation_key_after_created", statement: `UPDATE intents SET operation_key=$3, lifecycle_version=lifecycle_version+1 WHERE tenant_id=$1 AND intent_id=$2`, value: operation.OperationKey() + "changed"},
		{name: "operation_version_after_created", statement: `UPDATE intents SET operation_version=$3, lifecycle_version=lifecycle_version+1 WHERE tenant_id=$1 AND intent_id=$2`, value: int64(operation.Version() + 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, updateErr := integrationPool.Exec(context.Background(), test.statement, f.scope.TenantID(), draft.IntentID(), test.value)
			requirePostgresConstraint(t, updateErr)
		})
	}
}

func newDraftSibling(t *testing.T, base intents.Intent) intents.Intent {
	t.Helper()
	draft, err := intents.NewDraft(intents.Params{IntentID: unique("intent"), Version: 1, ClientRequestID: unique("client-request"), Nonce: unique("nonce"), Type: base.Type(), Ownership: base.Ownership(), Financial: base.Financial(), Route: base.Route(), Constraints: base.Constraints(), CreatedAt: base.CreatedAt(), ExpiresAt: base.ExpiresAt()})
	if err != nil {
		t.Fatal(err)
	}
	return draft
}
