package storage

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/intents"
)

type CreateIntentResult struct {
	Intent  intents.Intent
	Created bool
}

// IntentRepository persists frozen intent values. CreateIntent must treat an
// exact client-request replay as idempotent and reject changed material for the
// same request or intent identity. UpdateIntent uses optimistic lifecycle state.
type IntentRepository interface {
	FindIntentByID(context.Context, Scope, string) (intents.Intent, error)
	FindIntentByClientRequestID(context.Context, Scope, string) (intents.Intent, error)
	FindIntentByOperationKey(context.Context, Scope, string, uint64) (intents.Intent, error)
	CreateIntent(context.Context, Scope, intents.Intent) (CreateIntentResult, error)
	FreezeIntent(context.Context, Scope, intents.Intent, uint64) (intents.Intent, error)
	UpdateIntent(ctx context.Context, scope Scope, intent intents.Intent, expectedRevision uint64) (intents.Intent, error)
}
