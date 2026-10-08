// Package oauth owns the narrow Authorization Code + S256 control plane.
// It never authenticates humans, creates users, binds wallets, or grants signing authority.
package oauth

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
)

const (
	Issuer              = "https://connect.wizpay.xyz"
	Resource            = "https://mcp.wizpay.xyz/mcp"
	ResourceOrigin      = "https://mcp.wizpay.xyz"
	MetadataURL         = ResourceOrigin + "/.well-known/oauth-protected-resource/mcp"
	ReadScope           = "mcp:read"
	CodeLifetime        = 2 * time.Minute
	TransactionLifetime = 5 * time.Minute
	TokenLifetime       = 10 * time.Minute
)

var ErrInvalidClient = errors.New("oauth client denied")
var ErrDenied = errors.New("oauth authority denied")
var ErrBrowserUnavailable = errors.New("verified browser authentication and consent unavailable")
var opaqueID = regexp.MustCompile(`^[A-Za-z0-9._~-]{1,128}$`)
var verifierSyntax = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)

type Client struct {
	ID, Name, Type, AuthMethod, Status string
	RedirectURIs, Scopes, Resources    []string
}

func (c Client) Validate() error {
	if !opaqueID.MatchString(c.ID) || c.Name == "" || len(c.Name) > 200 || c.Type != "public" || c.AuthMethod != "none" || c.Status != "ACTIVE" || len(c.RedirectURIs) == 0 || len(c.RedirectURIs) > 8 || !slices.Equal(c.Scopes, []string{ReadScope}) || !slices.Equal(c.Resources, []string{Resource}) {
		return ErrDenied
	}
	for _, uri := range c.RedirectURIs {
		if !validRedirect(uri) {
			return ErrDenied
		}
	}
	return nil
}
func validRedirect(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && len(raw) <= 2048 && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == "" && u.RawFragment == "" && u.Opaque == "" && u.String() == raw && u.Hostname() != "localhost" && u.Host == strings.ToLower(u.Host) && !strings.ContainsAny(u.Host, "*\\")
}

type AuthorizationRequest struct{ ClientID, RedirectURI, Resource, Scope, ResponseType, Challenge, ChallengeMethod, State string }
type Transaction struct {
	ID                   string
	Request              AuthorizationRequest
	CreatedAt, ExpiresAt time.Time
}

// BrowserDecision is trusted port output, never HTTP input. SessionID is an opaque
// non-secret record reference. The future adapter must verify origin/CSRF, human
// session freshness/revocation and explicit consent for this exact transaction.
type BrowserDecision struct {
	Principal                                auth.AuthenticatedPrincipal
	SessionID                                string
	SessionExpiresAt                         time.Time
	TransactionID, ClientID, Resource, Scope string
}
type BrowserAuthorizer interface {
	AuthorizeBrowser(context.Context, Transaction) (BrowserDecision, error)
}
type Code struct {
	Digest, TransactionID, ConsentID, SessionID, TenantID, UserID string
	IssuedAt, ExpiresAt                                           time.Time
}
type Exchange struct{ ClientID, RedirectURI, Resource, Code, Verifier, GrantType string }
type Redemption struct {
	CodeDigest, Challenge, ClientID, RedirectURI, Resource, TokenDigest string
	Now, TokenExpiresAt                                                 time.Time
}
type Token struct {
	Digest, Issuer, Resource, ClientID, TenantID, UserID, IdentityIssuer, Subject, Scope, ConsentID, SessionID string
	IssuedAt, ExpiresAt                                                                                        time.Time
}
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}
type RevocationAuthorizer interface {
	AuthorizeRevocation(context.Context, Revocation) (BrowserDecision, error)
}
type Revocation struct {
	TenantID, UserID, ID string
	Kind                 string
}

// Repository operations must enforce relationships and atomic authority checks.
// No raw code, access token, session secret or PKCE verifier crosses this port.
type Repository interface {
	FindOAuthClient(context.Context, string) (Client, error)
	CreateOAuthTransaction(context.Context, Transaction) error
	FindOAuthTransaction(context.Context, string) (Transaction, error)
	CompleteOAuthAuthorization(context.Context, Transaction, BrowserDecision, Code) error
	RedeemOAuthCode(context.Context, Redemption) (Token, error)
	ValidateOAuthToken(context.Context, string, time.Time) (Token, error)
	RevokeOAuthAuthority(context.Context, Revocation) error
}
