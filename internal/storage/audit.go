package storage

import (
	"context"

	"github.com/deseti/wizpay-mcp/internal/audit"
)

type AuditRepository interface {
	AppendAudit(context.Context, Scope, audit.Record) error
	FindAuditByResource(context.Context, Scope, string, string) ([]audit.Record, error)
}
