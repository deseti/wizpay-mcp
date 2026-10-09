package browser

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/deseti/wizpay-mcp/internal/siwe"
	"strings"
	"time"
)

type Service struct {
	repo     Repository
	catalog  Catalog
	oauth    OAuth
	verifier IdentityVerifier
	wallet   *siwe.Service
	now      func() time.Time
}

func NewService(repo Repository, catalog Catalog, flow OAuth, verifier IdentityVerifier, now func() time.Time) (*Service, error) {
	if repo == nil || catalog == nil || flow == nil || now == nil {
		return nil, ErrDenied
	}
	return &Service{repo: repo, catalog: catalog, oauth: flow, verifier: verifier, now: now}, nil
}
func random(prefix string) (string, error) {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b[:]), nil
}
func csrf(raw string) string { return oauth.Digest("wizpay-browser-csrf-v1:" + raw) }
func validCredential(raw string) bool {
	if !strings.HasPrefix(raw, "wb_") || len(raw) != 46 {
		return false
	}
	b, e := base64.RawURLEncoding.DecodeString(raw[3:])
	return e == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == raw[3:]
}
func (s *Service) Start(ctx context.Context, t oauth.Transaction) (string, error) {
	tr, e := s.catalog.FindOAuthTransaction(ctx, t.ID)
	if e != nil || tr.Request != t.Request || !s.now().Before(tr.ExpiresAt) {
		return "", ErrDenied
	}
	raw, e := random("wb_")
	if e != nil {
		return "", e
	}
	ref, e := random("browser_")
	if e != nil {
		return "", e
	}
	now := s.now().UTC()
	expiry := now.Add(PendingLifetime)
	if tr.ExpiresAt.Before(expiry) {
		expiry = tr.ExpiresAt
	}
	e = s.repo.CreateBrowserSession(ctx, Session{Digest: oauth.Digest(raw), CSRFHash: oauth.Digest(csrf(raw)), Reference: ref, TransactionID: tr.ID, State: "PENDING", Decision: "OPEN", CreatedAt: now, ExpiresAt: expiry})
	return raw, e
}
func (s *Service) load(ctx context.Context, raw string) (Session, error) {
	if !validCredential(raw) {
		return Session{}, ErrDenied
	}
	v, e := s.repo.FindBrowserSession(ctx, oauth.Digest(raw))
	if e != nil || v.Revoked || !s.now().Before(v.ExpiresAt) || s.now().Before(v.CreatedAt) {
		return Session{}, ErrDenied
	}
	return v, nil
}
func (s *Service) proof(ctx context.Context, raw, token string) (Session, error) {
	v, e := s.load(ctx, raw)
	if e != nil || len(token) != 43 || subtle.ConstantTimeCompare([]byte(oauth.Digest(token)), []byte(v.CSRFHash)) != 1 {
		return Session{}, ErrDenied
	}
	return v, nil
}
func (s *Service) transaction(ctx context.Context, v Session) (oauth.Transaction, oauth.Client, error) {
	t, e := s.catalog.FindOAuthTransaction(ctx, v.TransactionID)
	if e != nil || !s.now().Before(t.ExpiresAt) || s.now().Before(t.CreatedAt) {
		return oauth.Transaction{}, oauth.Client{}, ErrDenied
	}
	c, e := s.catalog.FindOAuthClient(ctx, t.Request.ClientID)
	if e != nil || c.Validate() != nil || t.Request.Resource != oauth.Resource || t.Request.Scope != oauth.ReadScope {
		return oauth.Transaction{}, oauth.Client{}, ErrDenied
	}
	found := false
	for _, r := range c.RedirectURIs {
		if r == t.Request.RedirectURI {
			found = true
		}
	}
	if !found {
		return oauth.Transaction{}, oauth.Client{}, ErrDenied
	}
	return t, c, nil
}
func (s *Service) View(ctx context.Context, raw string) (View, error) {
	v, e := s.load(ctx, raw)
	if e != nil {
		return View{}, e
	}
	if v.Decision == "DENIED" {
		return View{State: "DENIED", CSRF: csrf(raw)}, nil
	}
	t, c, e := s.transaction(ctx, v)
	if e != nil {
		return View{}, e
	}
	user := ""
	if v.State == "AUTHENTICATED" {
		user = v.UserID
	}
	return View{State: v.State, ClientID: c.ID, ClientName: c.Name, Scope: t.Request.Scope, Resource: t.Request.Resource, UserID: user, CSRF: csrf(raw), AuthenticationAvailable: s.verifier != nil || s.wallet != nil}, nil
}

// Authenticate is an internal seam only; no WP3A HTTP endpoint invokes it.
func (s *Service) Authenticate(ctx context.Context, raw, token string) (string, error) {
	v, e := s.proof(ctx, raw, token)
	if e != nil {
		return "", e
	}
	if s.verifier == nil {
		return "", ErrUnavailable
	}
	if _, _, e = s.transaction(ctx, v); e != nil {
		return "", e
	}
	verified, e := s.verifier.VerifyBrowser(ctx, VerificationRequest{v.Reference, v.TransactionID})
	p := verified.Principal
	now := s.now().UTC()
	if e != nil || p.Validate() != nil || !p.ExpiresAt().After(now) || p.IssuedAt().After(now) || p.NotBefore().After(now) || verified.SessionReference != v.Reference || verified.TransactionID != v.TransactionID || verified.Evidence == "" || len(verified.Evidence) > 256 {
		return "", ErrDenied
	}
	if v.State == "AUTHENTICATED" && (v.TenantID != p.TenantID() || v.UserID != p.ActorID() || v.Issuer != p.IdentityProvider() || v.Subject != p.ProviderSubject()) {
		return "", ErrDenied
	}
	next, e := random("wb_")
	if e != nil {
		return "", e
	}
	ref, e := random("browser_")
	if e != nil {
		return "", e
	}
	expires := now.Add(SessionLifetime)
	if p.ExpiresAt().Before(expires) {
		expires = p.ExpiresAt()
	}
	n := Session{Digest: oauth.Digest(next), CSRFHash: oauth.Digest(csrf(next)), Reference: ref, TransactionID: v.TransactionID, State: "AUTHENTICATED", Decision: "OPEN", TenantID: p.TenantID(), UserID: p.ActorID(), Issuer: p.IdentityProvider(), Subject: p.ProviderSubject(), Evidence: verified.Evidence, CreatedAt: now, ExpiresAt: expires}
	if e = s.repo.RotateBrowserSession(ctx, v.Digest, v.CSRFHash, n); e != nil {
		return "", e
	}
	return next, nil
}

type authorizer struct {
	service    *Service
	raw, token string
}

func (a authorizer) AuthorizeBrowser(ctx context.Context, t oauth.Transaction) (oauth.BrowserDecision, error) {
	v, e := a.service.proof(ctx, a.raw, a.token)
	if e != nil || v.State != "AUTHENTICATED" || v.Decision != "OPEN" || v.TransactionID != t.ID {
		return oauth.BrowserDecision{}, ErrDenied
	}
	actual, _, e := a.service.transaction(ctx, v)
	if e != nil || actual.Request != t.Request {
		return oauth.BrowserDecision{}, ErrDenied
	}
	p, e := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: v.TenantID, ActorID: v.UserID, IdentityProvider: v.Issuer, ProviderSubject: v.Subject, ClientID: t.Request.ClientID, TokenID: v.Reference, IssuedAt: v.CreatedAt, NotBefore: v.CreatedAt, ExpiresAt: v.ExpiresAt})
	return oauth.BrowserDecision{Principal: p, SessionID: v.Reference, SessionExpiresAt: v.ExpiresAt, TransactionID: t.ID, ClientID: t.Request.ClientID, Resource: t.Request.Resource, Scope: t.Request.Scope, BrowserSessionReference: v.Reference}, e
}
func (s *Service) Grant(ctx context.Context, raw, token string) (string, error) {
	v, e := s.proof(ctx, raw, token)
	if e != nil {
		return "", e
	}
	if s.verifier == nil && s.wallet == nil {
		return "", ErrUnavailable
	}
	if v.State != "AUTHENTICATED" || v.Decision != "OPEN" {
		return "", ErrDenied
	}
	return s.oauth.CompleteRedirect(ctx, v.TransactionID, authorizer{s, raw, token})
}
func (s *Service) Deny(ctx context.Context, raw, token string) error {
	v, e := s.proof(ctx, raw, token)
	if e != nil {
		return e
	}
	return s.repo.DenyBrowserConsent(ctx, v.Digest, v.CSRFHash)
}
func (s *Service) Logout(ctx context.Context, raw, token string) error {
	v, e := s.proof(ctx, raw, token)
	if e != nil {
		return e
	}
	return s.repo.RevokeBrowserSession(ctx, v.Digest, v.CSRFHash)
}
