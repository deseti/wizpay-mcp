package requestauth

import (
	"context"
	"errors"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"net/http"
)

var errAuditRequired = errors.New("MCP access auditor is required")

// AccessRecord contains attribution only; never headers, tokens, bodies or URLs.
// AUTHORIZED is admission to MCP, not successful tool execution.
type AccessRecord struct{ RequestID, TenantID, UserID, ClientID, Method, Decision string }
type AccessAuditor interface {
	RecordMCPAccess(context.Context, AccessRecord) error
}

func NewAuditedOAuthMiddleware(verifier auth.TokenVerifier, resolver IdentityResolver, auditor AccessAuditor) (Middleware, error) {
	if auditor == nil {
		return Middleware{}, errAuditRequired
	}
	m, e := NewOAuthMiddleware(verifier, resolver)
	m.auditor = auditor
	return m, e
}
func (m Middleware) audit(r *http.Request, id, decision string, p auth.AuthenticatedPrincipal) error {
	if m.auditor == nil {
		return nil
	}
	method := r.Method
	switch method {
	case "GET", "POST", "DELETE", "OPTIONS":
	default:
		method = "OTHER"
	}
	return m.auditor.RecordMCPAccess(r.Context(), AccessRecord{RequestID: id, TenantID: p.TenantID(), UserID: p.ActorID(), ClientID: p.ClientID(), Method: method, Decision: decision})
}
