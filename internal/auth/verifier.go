package auth

import "context"

// TokenVerifier verifies a transport-extracted credential and returns only
// normalized trusted claims. Implementations must not retain the credential.
type TokenVerifier interface {
	Verify(context.Context, string) (AuthenticatedPrincipal, error)
}
