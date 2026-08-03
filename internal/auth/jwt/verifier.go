// Package jwt implements a narrow standards-based JWT verifier adapter. It
// uses configured key material and performs no discovery or network calls.
package jwt

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"

	"github.com/deseti/wizpay-mcp/internal/auth"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

type Config struct {
	Issuer            string
	Audience          string
	PublicKey         *rsa.PublicKey
	AllowedAlgorithms []string
	ClockSkew         time.Duration
}

// Claims is confined to the verifier boundary and normalized immediately.
type Claims struct {
	TenantID    string   `json:"tenant_id"`
	ActorID     string   `json:"actor_id"`
	ClientID    string   `json:"client_id,omitempty"`
	Permissions []string `json:"permissions"`
	jwtlib.RegisteredClaims
}

type Verifier struct {
	issuer, audience string
	publicKey        *rsa.PublicKey
	algorithms       []string
	clockSkew        time.Duration
	now              func() time.Time
}

const minimumRSAModulusBits = 2048

func ParseRSAPublicKey(data []byte) (*rsa.PublicKey, error) {
	block, rest := pem.Decode(data)
	if block == nil || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, fmt.Errorf("invalid authentication public key")
	}
	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PublicKey); ok {
			if !validRSAPublicKey(rsaKey) {
				return nil, fmt.Errorf("invalid authentication public key")
			}
			return rsaKey, nil
		}
	}
	if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		if !validRSAPublicKey(key) {
			return nil, fmt.Errorf("invalid authentication public key")
		}
		return key, nil
	}
	return nil, fmt.Errorf("invalid authentication public key")
}

func validRSAPublicKey(key *rsa.PublicKey) bool {
	return key != nil && key.N != nil && key.N.BitLen() >= minimumRSAModulusBits
}

func NewVerifier(config Config, now func() time.Time) (*Verifier, error) {
	config.Issuer, config.Audience = strings.TrimSpace(config.Issuer), strings.TrimSpace(config.Audience)
	if config.Issuer == "" || config.Audience == "" || !validRSAPublicKey(config.PublicKey) || len(config.AllowedAlgorithms) == 0 {
		return nil, fmt.Errorf("issuer, audience, public key, and allowed algorithms are required")
	}
	if now == nil || config.ClockSkew < 0 {
		return nil, fmt.Errorf("valid verifier clock and non-negative clock skew are required")
	}
	algorithms := make([]string, len(config.AllowedAlgorithms))
	for index, algorithm := range config.AllowedAlgorithms {
		algorithm = strings.TrimSpace(algorithm)
		if algorithm != jwtlib.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unsupported configured signing algorithm")
		}
		algorithms[index] = algorithm
	}
	return &Verifier{issuer: config.Issuer, audience: config.Audience, publicKey: config.PublicKey, algorithms: algorithms, clockSkew: config.ClockSkew, now: now}, nil
}

func (v *Verifier) Verify(ctx context.Context, credential string) (auth.AuthenticatedPrincipal, error) {
	if ctx == nil || v == nil || strings.TrimSpace(credential) == "" {
		return auth.AuthenticatedPrincipal{}, invalidCredential()
	}
	select {
	case <-ctx.Done():
		return auth.AuthenticatedPrincipal{}, invalidCredential()
	default:
	}
	claims := new(Claims)
	token, err := jwtlib.ParseWithClaims(credential, claims, func(token *jwtlib.Token) (any, error) {
		if token.Method.Alg() != jwtlib.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("signing method rejected")
		}
		return v.publicKey, nil
	}, jwtlib.WithValidMethods(v.algorithms), jwtlib.WithIssuer(v.issuer), jwtlib.WithAudience(v.audience), jwtlib.WithExpirationRequired(), jwtlib.WithIssuedAt(), jwtlib.WithLeeway(v.clockSkew), jwtlib.WithTimeFunc(v.now))
	if err != nil || token == nil || !token.Valid {
		return auth.AuthenticatedPrincipal{}, invalidCredential()
	}
	permissions := make([]auth.Permission, len(claims.Permissions))
	for index, permission := range claims.Permissions {
		permissions[index] = auth.Permission(strings.TrimSpace(permission))
	}
	params := auth.PrincipalParams{
		TenantID: claims.TenantID, ActorID: claims.ActorID, IdentityProvider: claims.Issuer, ProviderSubject: claims.Subject,
		ClientID: claims.ClientID, TokenID: claims.ID, Permissions: permissions,
	}
	if claims.IssuedAt != nil {
		params.IssuedAt = claims.IssuedAt.Time
	}
	if claims.NotBefore != nil {
		params.NotBefore = claims.NotBefore.Time
	}
	if claims.ExpiresAt != nil {
		params.ExpiresAt = claims.ExpiresAt.Time
	}
	principal, err := auth.NewAuthenticatedPrincipal(params)
	if err != nil {
		return auth.AuthenticatedPrincipal{}, invalidCredential()
	}
	return principal, nil
}

func invalidCredential() error {
	return apperrors.New(apperrors.CodeAuthenticationRequired, "Authentication is required.", false, true, false)
}

var _ auth.TokenVerifier = (*Verifier)(nil)
