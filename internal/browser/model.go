// Package browser owns browser-session authority independently of MCP tokens and
// financial human approval. WP3A provides no production identity verifier.
package browser

import (
	"context"
	"errors"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"time"
)

var ErrDenied = errors.New("browser authority denied")
var ErrUnavailable = errors.New("verified browser authentication unavailable")

const PendingLifetime = 5 * time.Minute
const SessionLifetime = 30 * time.Minute

type Session struct {
	Digest, CSRFHash, Reference, TransactionID, State, Decision string
	TenantID, UserID, Issuer, Subject, Evidence                 string
	CreatedAt, ExpiresAt                                        time.Time
	Revoked                                                     bool
}
type VerificationRequest struct{ SessionReference, TransactionID string }

// Verification is trusted adapter output, never HTTP input. WP3B must bind its
// cryptographic proof to the exact browser session and authorization transaction.
type Verification struct {
	Principal                                 auth.AuthenticatedPrincipal
	SessionReference, TransactionID, Evidence string
}
type IdentityVerifier interface {
	VerifyBrowser(context.Context, VerificationRequest) (Verification, error)
}
type Repository interface {
	CreateBrowserSession(context.Context, Session) error
	FindBrowserSession(context.Context, string) (Session, error)
	RotateBrowserSession(context.Context, string, string, Session) error
	RevokeBrowserSession(context.Context, string, string) error
	DenyBrowserConsent(context.Context, string, string) error
	CleanupBrowserSessions(context.Context) error
}
type OAuth interface {
	Begin(context.Context, oauth.AuthorizationRequest) (oauth.Transaction, error)
	CompleteRedirect(context.Context, string, oauth.BrowserAuthorizer) (string, error)
}
type Catalog interface {
	FindOAuthTransaction(context.Context, string) (oauth.Transaction, error)
	FindOAuthClient(context.Context, string) (oauth.Client, error)
}

// AuthenticationCorrelation exposes references, never credentials.
type AuthenticationCorrelation interface {
	FindBrowserAuthentication(context.Context, Session) (string, error)
}
type View struct {
	TransactionID           string `json:"transaction_id,omitempty"`
	ChallengeID             string `json:"challenge_id,omitempty"`
	State                   string `json:"state"`
	ClientID                string `json:"client_id"`
	ClientName              string `json:"client_name"`
	Scope                   string `json:"scope"`
	Resource                string `json:"resource"`
	UserID                  string `json:"user_id,omitempty"`
	CSRF                    string `json:"csrf"`
	AuthenticationAvailable bool   `json:"authentication_available"`
}
