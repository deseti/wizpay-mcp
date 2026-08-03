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
