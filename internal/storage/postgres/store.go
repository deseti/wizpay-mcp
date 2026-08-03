package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deseti/wizpay-mcp/internal/storage/postgres/dbsqlc"
)

type Store struct {
	pool         *pgxpool.Pool
	queries      *dbsqlc.Queries
	queryTimeout time.Duration
	logger       *slog.Logger
	now          func() time.Time
}

func NewStore(pool *pgxpool.Pool, queryTimeout time.Duration, logger *slog.Logger) (*Store, error) {
	if pool == nil || logger == nil || queryTimeout <= 0 {
		return nil, fmt.Errorf("valid pool, timeout, and logger are required")
	}
	return &Store{pool: pool, queries: dbsqlc.New(pool), queryTimeout: queryTimeout, logger: logger, now: time.Now}, nil
}

func (s *Store) queryContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("database context is required")
	}
	bounded, cancel := context.WithTimeout(ctx, s.queryTimeout)
	return bounded, cancel, nil
}
func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}
func (s *Store) Pool() *pgxpool.Pool {
	if s == nil {
		return nil
	}
	return s.pool
}
func (s *Store) Ping(ctx context.Context) error { return s.withTimeout(ctx, s.pool.Ping) }

func (s *Store) withTimeout(ctx context.Context, operation func(context.Context) error) error {
	if ctx == nil {
		return fmt.Errorf("database context is required")
	}
	bounded, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	return mapDatabaseError(operation(bounded))
}

func (s *Store) withTx(ctx context.Context, operation func(context.Context, *dbsqlc.Queries) error) error {
	if ctx == nil {
		return fmt.Errorf("database context is required")
	}
	bounded, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(bounded, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return mapDatabaseError(err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()
	if err := operation(bounded, s.queries.WithTx(tx)); err != nil {
		_ = tx.Rollback(bounded)
		return mapDatabaseError(err)
	}
	if err := tx.Commit(bounded); err != nil {
		return mapDatabaseError(err)
	}
	committed = true
	return nil
}
