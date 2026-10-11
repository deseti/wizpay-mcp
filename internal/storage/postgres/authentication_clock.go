package postgres

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

// AuthenticationTime shares PostgreSQL's authority clock with SIWE verification
// and owner approval. Zero on failure causes the domain's freshness checks to deny
// access. Never fall back to a different clock after a database error.
func (s *Store) AuthenticationTime() time.Time {
	ctx, cancel, e := s.queryContext(context.Background())
	if e != nil {
		return time.Time{}
	}
	defer cancel()
	var now time.Time
	if s.pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
		return time.Time{}
	}
	return now.UTC()
}
func (s *Store) UseDatabaseAuthenticationClock() {
	s.now = s.AuthenticationTime
	s.databaseAuthenticationClock = true
}

// Use the already-held transaction connection; never acquire another pool slot
// while holding authority locks (including a one-connection runtime pool).
func (s *Store) authenticationTimeIn(ctx context.Context, tx pgx.Tx) time.Time {
	if !s.databaseAuthenticationClock {
		return s.now().UTC()
	}
	now, e := dbWallTime(ctx, tx)
	if e != nil {
		return time.Time{}
	}
	return now.UTC()
}
