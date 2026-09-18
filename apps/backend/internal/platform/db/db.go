// Package db wraps pgxpool with Dzeroth-standard connection settings.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect creates a new pgxpool.Pool, applies Dzeroth pool settings, and verifies
// connectivity with Ping before returning. The caller owns the pool and must call
// Close when done.
//
// A 30-second context is applied to the dial and initial ping. If the database is
// unreachable within that window, Connect returns an error and the process should exit.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: parse dsn: %w", err)
	}

	cfg.MaxConns = 25
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(dialCtx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}

	pingCtx, pingCancel := context.WithTimeout(ctx, 30*time.Second)
	defer pingCancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	return pool, nil
}

// Close gracefully closes all connections in the pool.
func Close(pool *pgxpool.Pool) {
	if pool != nil {
		pool.Close()
	}
}
