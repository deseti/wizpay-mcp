package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// redacted is the only representation of an ephemeral user authorization that
// may ever reach a log, an error, or a serialized payload.
const redacted = "[REDACTED]"

// UserAuthorization carries the ephemeral, user-supplied session token that
// proves the user's own authorization to the provider.
//
// It is deliberately hostile to persistence and observability: the token is
// unexported, String and LogValue redact it, and MarshalJSON fails rather than
// emitting it. It must never be written to PostgreSQL, execution records,
// intents, approvals, policy state, audit metadata, or logs. Only Reveal, at
// the outbound provider HTTP boundary, returns the raw value.
type UserAuthorization struct {
	token string
}

// NewUserAuthorization validates an ephemeral user session token. The token is
// checked for transport safety only; its authenticity is the provider's
// decision, never this layer's.
func NewUserAuthorization(token string) (UserAuthorization, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return UserAuthorization{}, fmt.Errorf("user authorization token is required")
	}
	if len(trimmed) > 4096 {
		return UserAuthorization{}, fmt.Errorf("user authorization token is too large")
	}
	for _, character := range trimmed {
		if character < 0x21 || character > 0x7E {
			return UserAuthorization{}, fmt.Errorf("user authorization token contains unsupported characters")
		}
	}
	return UserAuthorization{token: trimmed}, nil
}

// Present reports whether a token is held. It never reveals the token.
func (a UserAuthorization) Present() bool { return a.token != "" }

// Reveal returns the raw token. The only permitted caller is the outbound
// provider request builder. The result must never be logged, persisted,
// wrapped into an error, or copied into a domain type.
func (a UserAuthorization) Reveal() string { return a.token }

func (a UserAuthorization) String() string { return redacted }

func (a UserAuthorization) GoString() string { return redacted }

func (a UserAuthorization) LogValue() slog.Value { return slog.StringValue(redacted) }

// MarshalJSON fails closed so an ephemeral token can never be serialized into
// a stored payload or a structured log record.
func (a UserAuthorization) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("user authorization must never be serialized")
}

var (
	_ fmt.Stringer   = UserAuthorization{}
	_ slog.LogValuer = UserAuthorization{}
	_ json.Marshaler = UserAuthorization{}
)

// AuthorizationSource supplies the ephemeral user authorization for one
// execution at provider-call time.
//
// Absence is not an error: it means the user has not currently delegated an
// authorized session, so the execution must wait for user action rather than
// fail. Backend orchestration must never synthesize this value — holding a
// WizPay session is not equivalent to holding Circle signing authority.
type AuthorizationSource interface {
	UserAuthorization(ctx context.Context, executionID string) (UserAuthorization, bool, error)
}

type authorizationContextKey struct{}

// WithUserAuthorization scopes an ephemeral user authorization to a request
// context. It is never stored beyond the lifetime of that context.
func WithUserAuthorization(ctx context.Context, executionID string, authorization UserAuthorization) context.Context {
	if executionID == "" || !authorization.Present() {
		return ctx
	}
	existing, _ := ctx.Value(authorizationContextKey{}).(map[string]UserAuthorization)
	scoped := make(map[string]UserAuthorization, len(existing)+1)
	for key, value := range existing {
		scoped[key] = value
	}
	scoped[executionID] = authorization
	return context.WithValue(ctx, authorizationContextKey{}, scoped)
}

// ContextAuthorizationSource reads the ephemeral authorization that the calling
// user attached to the current request context.
type ContextAuthorizationSource struct{}

func (ContextAuthorizationSource) UserAuthorization(ctx context.Context, executionID string) (UserAuthorization, bool, error) {
	if executionID == "" {
		return UserAuthorization{}, false, fmt.Errorf("execution ID is required")
	}
	scoped, _ := ctx.Value(authorizationContextKey{}).(map[string]UserAuthorization)
	authorization, found := scoped[executionID]
	if !found || !authorization.Present() {
		return UserAuthorization{}, false, nil
	}
	return authorization, true, nil
}

var _ AuthorizationSource = ContextAuthorizationSource{}
