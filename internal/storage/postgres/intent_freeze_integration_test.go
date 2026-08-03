package postgres

import (
	"context"
	"testing"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

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
