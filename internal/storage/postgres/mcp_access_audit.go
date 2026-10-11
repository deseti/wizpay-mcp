package postgres

import (
	"context"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
)

func (s *Store) RecordMCPAccess(ctx context.Context, r requestauth.AccessRecord) error {
	bounded, cancel, e := s.queryContext(ctx)
	if e != nil {
		return e
	}
	defer cancel()
	_, e = s.pool.Exec(bounded, `INSERT INTO mcp_access_audit(request_id,tenant_id,user_id,client_id,http_method,decision) VALUES($1,$2,$3,$4,$5,$6)`, r.RequestID, r.TenantID, r.UserID, r.ClientID, r.Method, r.Decision)
	return e
}
