package storage

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Scope is trusted application context for every tenant-owned repository
// operation. MCP input must never construct it directly.
type Scope struct {
	tenantID  string
	actorID   string
	requestID string
	traceID   string
}

func NewScope(tenantID, actorID, requestID, traceID string) (Scope, error) {
	scope := Scope{tenantID: strings.TrimSpace(tenantID), actorID: strings.TrimSpace(actorID), requestID: strings.TrimSpace(requestID), traceID: strings.TrimSpace(traceID)}
	for name, value := range map[string]string{"tenant ID": scope.tenantID, "actor ID": scope.actorID, "request ID": scope.requestID} {
		if err := validateScopeField(name, value, true); err != nil {
			return Scope{}, err
		}
	}
	if err := validateScopeField("trace ID", scope.traceID, false); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

func (s Scope) Validate() error {
	_, err := NewScope(s.tenantID, s.actorID, s.requestID, s.traceID)
	return err
}
func (s Scope) TenantID() string  { return s.tenantID }
func (s Scope) ActorID() string   { return s.actorID }
func (s Scope) RequestID() string { return s.requestID }
func (s Scope) TraceID() string   { return s.traceID }

type scopeContextKey struct{}

// WithScope attaches the trusted persistence scope to a context so components
// reached through scope-free ports can recover it. The Phase 9 execution
// adapter contract identifies work by execution ID alone, so a provider adapter
// that must read its own persisted state has no other way to stay
// tenant-scoped.
//
// The scope carries no credential and grants no authority on its own: it is
// still validated by every repository that receives it.
func WithScope(ctx context.Context, scope Scope) context.Context {
	if ctx == nil || scope.Validate() != nil {
		return ctx
	}
	return context.WithValue(ctx, scopeContextKey{}, scope)
}

// ScopeFromContext returns the scope attached by WithScope. A missing scope is
// reported rather than substituted, so no caller can fall back to an
// unscoped read.
func ScopeFromContext(ctx context.Context) (Scope, bool) {
	if ctx == nil {
		return Scope{}, false
	}
	scope, found := ctx.Value(scopeContextKey{}).(Scope)
	if !found || scope.Validate() != nil {
		return Scope{}, false
	}
	return scope, true
}

func validateScopeField(name, value string, required bool) error {
	if required && value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if value == "" {
		return nil
	}
	if len(value) > 256 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s is invalid", name)
	}
	return nil
}

type Tenant struct {
	TenantID  string
	CreatedAt time.Time
}

func (t Tenant) Validate() error {
	if err := validateScopeField("tenant ID", strings.TrimSpace(t.TenantID), true); err != nil {
		return err
	}
	if t.CreatedAt.IsZero() {
		return fmt.Errorf("tenant creation time is required")
	}
	return nil
}

type TenantRepository interface {
	CreateTenant(context.Context, Tenant) (Tenant, error)
}
