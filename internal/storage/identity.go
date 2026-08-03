// Package storage defines persistence contracts only. Phase 2 provides no
// implementation and performs no I/O.
package storage

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/auth"
)

// IdentityRepository resolves previously established application identities.
// Implementations must preserve auth.Identity validation and lifecycle rules.
type IdentityRepository interface {
	CreateIdentity(context.Context, Scope, auth.Identity) (auth.Identity, error)
	FindIdentityByID(context.Context, Scope, string) (auth.Identity, error)
}
