package execution

import "context"

// Adapter is the provider-neutral future execution boundary. Phase 5 provides
// no implementation.
type Adapter interface {
	Execute(context.Context, Request) (Result, error)
	GetStatus(context.Context, string) (Result, error)
}
