package auth

import "context"

// HumanAuthenticator is a separate browser authentication boundary. An implementation
// must validate human-session authority independently of MCP bearer claims.
// WP1 deliberately supplies no production implementation.
type HumanAuthenticator interface {
	AuthenticateHuman(context.Context) (TrustedRequest, error)
}
type humanContextKey struct{}

// AuthenticateHumanContext calls the trusted authentication port; it accepts no
// caller-supplied human flag. Never wire the MCP TokenVerifier into this port.
func AuthenticateHumanContext(ctx context.Context, verifier HumanAuthenticator) (context.Context, error) {
	if ctx == nil || verifier == nil {
		return ctx, authorizationDenied()
	}
	human, err := verifier.AuthenticateHuman(ctx)
	if err != nil || human.Validate() != nil {
		return ctx, authorizationDenied()
	}
	current, err := TrustedRequestFromContext(ctx)
	if err != nil || !sameAuthority(current, human) {
		return ctx, authorizationDenied()
	}
	return context.WithValue(ctx, humanContextKey{}, human), nil
}
func sameAuthority(a, b TrustedRequest) bool {
	x, y := a.Principal(), b.Principal()
	return x.TenantID() == y.TenantID() && x.ActorID() == y.ActorID() && x.IdentityProvider() == y.IdentityProvider() && x.ProviderSubject() == y.ProviderSubject() && x.ClientID() == y.ClientID() && x.TokenID() == y.TokenID()
}

// RequireHuman is checked at both transport and application boundaries.
func RequireHuman(ctx context.Context, permission Permission) error {
	request, err := TrustedRequestFromContext(ctx)
	if err != nil {
		return err
	}
	human, ok := ctx.Value(humanContextKey{}).(TrustedRequest)
	if !ok || human.Validate() != nil || !sameAuthority(request, human) || !human.Principal().HasPermission(permission) {
		return authorizationDenied()
	}
	return nil
}

// VerifiedAgentID derives the acting identity from verified claims, never metadata
// or tool arguments. Future OAuth must bind this identifier to client registration.
func VerifiedAgentID(request TrustedRequest) (string, error) {
	if request.Validate() != nil || request.Principal().ClientID() == "" {
		return "", authorizationDenied()
	}
	return request.Principal().ClientID(), nil
}
