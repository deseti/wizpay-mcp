package requestauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/oauth"
)

// IdentityResolver loads the already persisted WizPay identity. It must not
// provision or activate identities during an ordinary authenticated request.
type IdentityResolver interface {
	ResolveIdentity(context.Context, auth.AuthenticatedPrincipal) (auth.Identity, error)
}

// ResolveIdentityFunc adapts a function for tests and application wiring.
type ResolveIdentityFunc func(context.Context, auth.AuthenticatedPrincipal) (auth.Identity, error)

func (f ResolveIdentityFunc) ResolveIdentity(ctx context.Context, principal auth.AuthenticatedPrincipal) (auth.Identity, error) {
	return f(ctx, principal)
}

// Middleware authenticates only the protected handler; it does not inspect MCP
// arguments and never passes a raw bearer token downstream.
type Middleware struct {
	verifier auth.TokenVerifier
	resolver IdentityResolver
	oauth    bool
}

func NewMiddleware(verifier auth.TokenVerifier, resolver IdentityResolver) (Middleware, error) {
	if verifier == nil || resolver == nil {
		return Middleware{}, fmt.Errorf("token verifier and identity resolver are required")
	}
	return Middleware{verifier: verifier, resolver: resolver}, nil
}

// NewOAuthMiddleware rejects legacy JWT/browser credentials through the supplied
// OAuth verifier and supplies protected-resource discovery in bearer challenges.
func NewOAuthMiddleware(verifier auth.TokenVerifier, resolver IdentityResolver) (Middleware, error) {
	m, e := NewMiddleware(verifier, resolver)
	m.oauth = true
	return m, e
}
func (m Middleware) unauthorized(w http.ResponseWriter, invalid bool) {
	if !m.oauth {
		writeUnauthorized(w)
		return
	}
	value := `Bearer resource_metadata="` + oauth.MetadataURL + `", scope="` + oauth.ReadScope + `"`
	if invalid {
		value += `, error="invalid_token"`
	}
	w.Header().Set("WWW-Authenticate", value)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
}
func (m Middleware) Wrap(next http.Handler) http.Handler { return m.Handler(next) }

func (m Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if len(request.Header.Values("Authorization")) > 1 || (m.oauth && (request.Header.Get("Cookie") != "" || len(request.Header.Get("Authorization")) > 256)) {
			m.unauthorized(response, true)
			return
		}
		credential, ok := bearerCredential(request.Header.Get("Authorization"))
		if !ok {
			m.unauthorized(response, request.Header.Get("Authorization") != "")
			return
		}
		principal, err := m.verifier.Verify(request.Context(), credential)
		if err != nil {
			m.unauthorized(response, true)
			return
		}
		if m.oauth && (!principal.HasPermission(auth.PermissionReadIntent) || !principal.HasPermission(auth.PermissionReadApproval)) {
			response.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+oauth.MetadataURL+`", error="insufficient_scope", scope="`+oauth.ReadScope+`"`)
			response.Header().Set("Cache-Control", "no-store")
			response.WriteHeader(http.StatusForbidden)
			return
		}
		identity, err := m.resolver.ResolveIdentity(request.Context(), principal)
		if err != nil {
			m.unauthorized(response, true)
			return
		}
		metadata, err := serverMetadata(request)
		if err != nil {
			m.unauthorized(response, true)
			return
		}
		trusted, err := auth.NewTrustedRequest(principal, identity, metadata)
		if err != nil {
			m.unauthorized(response, true)
			return
		}
		downstream := request.Clone(auth.WithTrustedRequest(request.Context(), trusted))
		downstream.Header = request.Header.Clone()
		downstream.Header.Del("Authorization")
		next.ServeHTTP(response, downstream)
	})
}

func bearerCredential(value string) (string, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func serverMetadata(request *http.Request) (auth.RequestMetadata, error) {
	requestID, err := randomID()
	if err != nil {
		return auth.RequestMetadata{}, err
	}
	traceID, err := randomID()
	if err != nil {
		return auth.RequestMetadata{}, err
	}
	return auth.RequestMetadata{RequestID: requestID, TraceID: traceID}, nil
}

func randomID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func writeUnauthorized(response http.ResponseWriter) {
	response.Header().Set("WWW-Authenticate", `Bearer realm="wizpay-mcp"`)
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(http.StatusUnauthorized)
}
