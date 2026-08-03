package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, cfg Config, logger *slog.Logger) (*Store, error) {
	if ctx == nil {
		return nil, fmt.Errorf("database context is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("database logger is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid database configuration")
	}
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration")
	}
	poolConfig.MaxConns, poolConfig.MinConns = cfg.MaxConnections, cfg.MinConnections
	connectContext, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(connectContext, poolConfig)
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	if err := pool.Ping(connectContext); err != nil {
		pool.Close()
		return nil, mapDatabaseError(err)
	}
	logger.Info("database_connected", "max_connections", cfg.MaxConnections, "min_connections", cfg.MinConnections)
	return NewStore(pool, cfg.QueryTimeout, logger)
}
