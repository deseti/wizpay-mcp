package auth

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// PrincipalParams contains normalized claims produced by a credential verifier.
// It must never contain a raw credential or an untyped claims map.
type PrincipalParams struct {
	TenantID         string
	ActorID          string
	IdentityProvider string
	ProviderSubject  string
	ClientID         string
	TokenID          string
	AuthenticatedAt  time.Time
	IssuedAt         time.Time
	NotBefore        time.Time
	ExpiresAt        time.Time
	Permissions      []Permission
}

// AuthenticatedPrincipal is immutable trusted authentication output.
type AuthenticatedPrincipal struct {
	tenantID, actorID, identityProvider, providerSubject string
	clientID, tokenID                                    string
	authenticatedAt, issuedAt, notBefore, expiresAt      time.Time
	permissions                                          []Permission
}

func NewAuthenticatedPrincipal(params PrincipalParams) (AuthenticatedPrincipal, error) {
	p := AuthenticatedPrincipal{
		tenantID: strings.TrimSpace(params.TenantID), actorID: strings.TrimSpace(params.ActorID),
		identityProvider: strings.TrimSpace(params.IdentityProvider), providerSubject: strings.TrimSpace(params.ProviderSubject),
		clientID: strings.TrimSpace(params.ClientID), tokenID: strings.TrimSpace(params.TokenID),
		authenticatedAt: params.AuthenticatedAt.UTC(), issuedAt: params.IssuedAt.UTC(),
		notBefore: params.NotBefore.UTC(), expiresAt: params.ExpiresAt.UTC(),
	}
	for name, value := range map[string]string{
		"tenant ID": p.tenantID, "actor ID": p.actorID, "identity provider": p.identityProvider, "provider subject": p.providerSubject,
	} {
		if err := validateIdentityField(name, value); err != nil {
			return AuthenticatedPrincipal{}, err
		}
	}
	for name, value := range map[string]string{"client ID": p.clientID, "token ID": p.tokenID} {
		if value != "" {
			if err := validateIdentityField(name, value); err != nil {
				return AuthenticatedPrincipal{}, err
			}
		}
	}
	if p.expiresAt.IsZero() {
		return AuthenticatedPrincipal{}, fmt.Errorf("credential expiration is required")
	}
	seen := make(map[Permission]struct{}, len(params.Permissions))
	for _, permission := range params.Permissions {
		if err := validatePermission(permission); err != nil {
			return AuthenticatedPrincipal{}, err
		}
		seen[permission] = struct{}{}
	}
	p.permissions = make([]Permission, 0, len(seen))
	for permission := range seen {
		p.permissions = append(p.permissions, permission)
	}
	sort.Slice(p.permissions, func(i, j int) bool { return p.permissions[i] < p.permissions[j] })
	return p, nil
}

func (p AuthenticatedPrincipal) Validate() error {
	_, err := NewAuthenticatedPrincipal(PrincipalParams{TenantID: p.tenantID, ActorID: p.actorID, IdentityProvider: p.identityProvider, ProviderSubject: p.providerSubject, ClientID: p.clientID, TokenID: p.tokenID, AuthenticatedAt: p.authenticatedAt, IssuedAt: p.issuedAt, NotBefore: p.notBefore, ExpiresAt: p.expiresAt, Permissions: p.permissions})
	return err
}
func (p AuthenticatedPrincipal) TenantID() string           { return p.tenantID }
func (p AuthenticatedPrincipal) ActorID() string            { return p.actorID }
func (p AuthenticatedPrincipal) IdentityProvider() string   { return p.identityProvider }
func (p AuthenticatedPrincipal) ProviderSubject() string    { return p.providerSubject }
func (p AuthenticatedPrincipal) ClientID() string           { return p.clientID }
func (p AuthenticatedPrincipal) TokenID() string            { return p.tokenID }
func (p AuthenticatedPrincipal) AuthenticatedAt() time.Time { return p.authenticatedAt }
func (p AuthenticatedPrincipal) IssuedAt() time.Time        { return p.issuedAt }
func (p AuthenticatedPrincipal) NotBefore() time.Time       { return p.notBefore }
func (p AuthenticatedPrincipal) ExpiresAt() time.Time       { return p.expiresAt }
func (p AuthenticatedPrincipal) Permissions() []Permission {
	return append([]Permission(nil), p.permissions...)
}
func (p AuthenticatedPrincipal) HasPermission(permission Permission) bool {
	index := sort.Search(len(p.permissions), func(i int) bool { return p.permissions[i] >= permission })
	return index < len(p.permissions) && p.permissions[index] == permission
}
