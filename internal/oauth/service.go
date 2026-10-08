package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository, now func() time.Time) (*Service, error) {
	if repo == nil || now == nil {
		return nil, ErrDenied
	}
	return &Service{repo: repo, now: now}, nil
}
func random(prefix string) (string, error) {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b[:]), nil
}
func Digest(value string) string {
	s := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(s[:])
}
func Permissions(scope string) ([]auth.Permission, error) {
	if scope != ReadScope {
		return nil, ErrDenied
	}
	return []auth.Permission{auth.PermissionReadIntent, auth.PermissionReadApproval}, nil
}
func validChallenge(challenge string) bool {
	b, e := base64.RawURLEncoding.DecodeString(challenge)
	return e == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == challenge
}

// CompleteRedirect implements exact registered redirect/state and RFC9207 issuer
// binding for the future browser adapter. No HTTP route invokes this in WP2.
func (s *Service) CompleteRedirect(ctx context.Context, id string, browser BrowserAuthorizer) (string, error) {
	t, e := s.repo.FindOAuthTransaction(ctx, id)
	if e != nil {
		return "", ErrDenied
	}
	code, e := s.Complete(ctx, id, browser)
	if e != nil {
		return "", e
	}
	u, e := url.Parse(t.Request.RedirectURI)
	if e != nil {
		return "", ErrDenied
	}
	q := u.Query()
	q.Set("code", code)
	q.Set("iss", Issuer)
	if t.Request.State != "" {
		q.Set("state", t.Request.State)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func (s *Service) Begin(ctx context.Context, r AuthorizationRequest) (Transaction, error) {
	if r.ResponseType != "code" || r.Resource != Resource || r.Scope != ReadScope || r.ChallengeMethod != "S256" || !validChallenge(r.Challenge) || len(r.State) > 512 || strings.ContainsAny(r.State, "\r\n\x00") {
		return Transaction{}, ErrDenied
	}
	c, e := s.repo.FindOAuthClient(ctx, r.ClientID)
	if e != nil || c.Validate() != nil || !slices.Contains(c.RedirectURIs, r.RedirectURI) {
		return Transaction{}, ErrDenied
	}
	id, e := random("tx_")
	if e != nil {
		return Transaction{}, e
	}
	now := s.now().UTC()
	t := Transaction{ID: id, Request: r, CreatedAt: now, ExpiresAt: now.Add(TransactionLifetime)}
	return t, s.repo.CreateOAuthTransaction(ctx, t)
}

// Complete is intentionally not wired to any HTTP endpoint in WP2. Only a future
// separately reviewed browser/consent adapter may supply this port.
func (s *Service) Complete(ctx context.Context, id string, browser BrowserAuthorizer) (string, error) {
	if browser == nil {
		return "", ErrBrowserUnavailable
	}
	t, e := s.repo.FindOAuthTransaction(ctx, id)
	if e != nil || !s.now().Before(t.ExpiresAt) {
		return "", ErrDenied
	}
	d, e := browser.AuthorizeBrowser(ctx, t)
	if e != nil {
		return "", ErrDenied
	}
	p := d.Principal
	now := s.now().UTC()
	if p.Validate() != nil || !p.ExpiresAt().After(now) || p.IssuedAt().After(now) || p.NotBefore().After(now) || d.SessionID == "" || len(d.SessionID) > 128 || !d.SessionExpiresAt.After(now) || d.TransactionID != t.ID || d.ClientID != t.Request.ClientID || d.Resource != Resource || d.Scope != t.Request.Scope {
		return "", ErrDenied
	}
	raw, e := random("wmcp_ac_")
	if e != nil {
		return "", e
	}
	consent, e := random("consent_")
	if e != nil {
		return "", e
	}
	code := Code{Digest: Digest(raw), TransactionID: id, ConsentID: consent, SessionID: d.SessionID, TenantID: p.TenantID(), UserID: p.ActorID(), IssuedAt: now, ExpiresAt: minTime(now.Add(CodeLifetime), t.ExpiresAt, d.SessionExpiresAt, p.ExpiresAt())}
	if e = s.repo.CompleteOAuthAuthorization(ctx, t, d, code); e != nil {
		return "", e
	}
	return raw, nil
}
func minTime(t time.Time, rest ...time.Time) time.Time {
	for _, v := range rest {
		if v.Before(t) {
			t = v
		}
	}
	return t
}
func (s *Service) Exchange(ctx context.Context, x Exchange) (TokenResponse, error) {
	if x.GrantType != "authorization_code" || x.Resource != Resource || !strings.HasPrefix(x.Code, "wmcp_ac_") || len(x.Code) != 51 || !verifierSyntax.MatchString(x.Verifier) || !validRedirect(x.RedirectURI) {
		return TokenResponse{}, ErrDenied
	}
	client, e := s.repo.FindOAuthClient(ctx, x.ClientID)
	if e != nil || client.Validate() != nil {
		return TokenResponse{}, ErrInvalidClient
	}
	if !slices.Contains(client.RedirectURIs, x.RedirectURI) {
		return TokenResponse{}, ErrDenied
	}
	raw, e := random("wmcp_at_")
	if e != nil {
		return TokenResponse{}, e
	}
	now := s.now().UTC()
	t, e := s.repo.RedeemOAuthCode(ctx, Redemption{CodeDigest: Digest(x.Code), Challenge: Digest(x.Verifier), ClientID: x.ClientID, RedirectURI: x.RedirectURI, Resource: x.Resource, TokenDigest: Digest(raw), Now: now, TokenExpiresAt: now.Add(TokenLifetime)})
	if e != nil {
		return TokenResponse{}, ErrDenied
	}
	return TokenResponse{AccessToken: raw, TokenType: "Bearer", ExpiresIn: int(t.ExpiresAt.Sub(s.now().UTC()).Seconds()), Scope: t.Scope}, nil
}

// Verify implements the existing TokenVerifier port. Opaque token rows bind the
// authorization-server issuer independently of the user's authentication issuer.
func (s *Service) Verify(ctx context.Context, raw string) (auth.AuthenticatedPrincipal, error) {
	if !strings.HasPrefix(raw, "wmcp_at_") || len(raw) != 51 {
		return auth.AuthenticatedPrincipal{}, ErrDenied
	}
	t, e := s.repo.ValidateOAuthToken(ctx, Digest(raw), s.now().UTC())
	if e != nil || t.Issuer != Issuer || t.Resource != Resource || t.ClientID == "" || !t.ExpiresAt.After(s.now()) || t.IssuedAt.After(s.now()) {
		return auth.AuthenticatedPrincipal{}, ErrDenied
	}
	permissions, e := Permissions(t.Scope)
	if e != nil {
		return auth.AuthenticatedPrincipal{}, e
	}
	return auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: t.TenantID, ActorID: t.UserID, IdentityProvider: t.IdentityIssuer, ProviderSubject: t.Subject, ClientID: t.ClientID, TokenID: t.Digest, IssuedAt: t.IssuedAt, NotBefore: t.IssuedAt, ExpiresAt: t.ExpiresAt, Permissions: permissions})
}

// Revoke is an internal user-scoped seam, not an MCP tool or public endpoint.
// The caller must first establish the independent browser identity boundary.
func (s *Service) Revoke(ctx context.Context, browser RevocationAuthorizer, r Revocation) error {
	if browser == nil {
		return ErrBrowserUnavailable
	}
	d, e := browser.AuthorizeRevocation(ctx, r)
	if e != nil || d.Principal.Validate() != nil || !d.Principal.ExpiresAt().After(s.now()) || d.Principal.NotBefore().After(s.now()) || d.Principal.IssuedAt().After(s.now()) || !d.SessionExpiresAt.After(s.now()) || d.SessionID == "" || d.Principal.TenantID() != r.TenantID || d.Principal.ActorID() != r.UserID {
		return ErrDenied
	}
	return s.repo.RevokeOAuthAuthority(ctx, r)
}

var _ auth.TokenVerifier = (*Service)(nil)
