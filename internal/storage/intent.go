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
	FindIntentByID(context.Context, string) (intents.Intent, error)
	FindIntentByClientRequestID(context.Context, string) (intents.Intent, error)
	FindIntentByOperationKey(context.Context, string, uint64) (intents.Intent, error)
	CreateIntent(context.Context, intents.Intent) (CreateIntentResult, error)
	UpdateIntent(ctx context.Context, intent intents.Intent, expectedStatus intents.Status) (intents.Intent, error)
}
