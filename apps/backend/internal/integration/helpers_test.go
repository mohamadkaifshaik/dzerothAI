//go:build integration

// Package integration contains live integration tests that run against real
// PostgreSQL and Redis services. These tests require running service containers
// and are gated behind the "integration" build tag so they never execute during
// the normal unit-test pass (go test ./...).
//
// Run with: go test -tags integration ./internal/integration/
//
// Required environment variables:
//
//	POSTGRES_USER      — database user (default: dzeroth)
//	POSTGRES_PASSWORD  — database password (default: cipassword)
//	POSTGRES_HOST      — database host (default: localhost)
//	POSTGRES_PORT      — database port (default: 5432)
//	POSTGRES_DB        — database name (default: dzeroth_test)
//	REDIS_ADDR         — Redis address host:port (default: localhost:6379)
//	REDIS_PASSWORD     — Redis AUTH password (default: ciredispassword)
package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	rdb "github.com/redis/go-redis/v9"

	appdb "github.com/mohamadkaifshaik/dzerothAI/apps/backend/db"
	platformDB "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/db"
	platformRedis "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/redis"
)

// envOrDefault returns the value of the environment variable key, or def if unset/empty.
func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// testPostgresDSN builds the DSN for the integration test database from
// environment variables, falling back to CI defaults.
func testPostgresDSN() string {
	user := envOrDefault("POSTGRES_USER", "dzeroth")
	password := envOrDefault("POSTGRES_PASSWORD", "cipassword")
	host := envOrDefault("POSTGRES_HOST", "localhost")
	port := envOrDefault("POSTGRES_PORT", "5432")
	dbName := envOrDefault("POSTGRES_DB", "dzeroth_test")
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, dbName)
}

// testRedisAddr returns the Redis address for the integration test service,
// falling back to the CI default.
func testRedisAddr() string {
	return envOrDefault("REDIS_ADDR", "localhost:6379")
}

// testRedisPassword returns the Redis AUTH password for the integration test
// service. The CI job configures Redis with --requirepass ciredispassword.
func testRedisPassword() string {
	return envOrDefault("REDIS_PASSWORD", "ciredispassword")
}

// connectTestDB connects to the integration test PostgreSQL database and runs
// all pending migrations. The returned pool must be closed by the caller via
// pool.Close() (or t.Cleanup).
func connectTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := testPostgresDSN()
	pool, err := platformDB.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("integration: connect postgres: %v", err)
	}

	if err := runTestMigrations(dsn); err != nil {
		pool.Close()
		t.Fatalf("integration: run migrations: %v", err)
	}

	t.Cleanup(func() { pool.Close() })
	return pool
}

// runTestMigrations applies all pending migrations to the test database using
// the embedded migration files from appdb.MigrationsFS.
func runTestMigrations(dsn string) error {
	src, err := iofs.New(appdb.MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}

	// golang-migrate's pgx/v5 driver requires "pgx5://" prefix.
	migrateDSN := "pgx5://" + strings.TrimPrefix(dsn, "postgres://")

	m, err := migrate.NewWithSourceInstance("iofs", src, migrateDSN)
	if err != nil {
		return fmt.Errorf("migration init: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration up: %w", err)
	}
	return nil
}

// connectTestRedis connects to the integration test Redis service using the
// configured address and password. The returned client must be closed by the caller.
func connectTestRedis(t *testing.T) *rdb.Client {
	t.Helper()

	client, err := platformRedis.Connect(context.Background(), testRedisAddr(), testRedisPassword(), false)
	if err != nil {
		t.Fatalf("integration: connect redis: %v", err)
	}

	t.Cleanup(func() { _ = platformRedis.Close(client) })
	return client
}
