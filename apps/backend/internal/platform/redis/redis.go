// Package redis wraps go-redis with Dzeroth-standard connection helpers.
package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// buildOptions constructs the redis.Options from the provided parameters.
// Extracted so that tests can verify option construction without dialing.
func buildOptions(addr, password string, useTLS bool) *redis.Options {
	opts := &redis.Options{
		Addr:     addr,
		Password: password,
	}
	if useTLS {
		opts.TLSConfig = &tls.Config{} // system CA pool; MinVersion defaults to TLS 1.2
	}
	return opts
}

// Connect creates a new Redis client, verifies connectivity with Ping, and returns the
// client. If Ping fails the error is returned but callers may choose to log and continue
// (Redis availability is required only at the rate-limiting boundary, not at startup).
//
// password is sent as the Redis AUTH credential; pass an empty string for unauthenticated
// connections (e.g. local development). useTLS enables TLS using the system's default CA
// pool — suitable for managed Redis providers (AWS ElastiCache, GCP Memorystore, etc.).
//
// The password is never logged.
func Connect(ctx context.Context, addr, password string, useTLS bool) (*redis.Client, error) {
	opts := buildOptions(addr, password, useTLS)
	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: ping %s: %w", addr, err)
	}

	return client, nil
}

// Close shuts down the Redis client connection.
func Close(client *redis.Client) error {
	if client == nil {
		return nil
	}
	return client.Close()
}

// IsAvailable performs a non-blocking availability check. It returns false if the Ping
// round-trip fails or exceeds 1 second. Used by rate-limiting middleware to decide
// whether to fail closed (503) or proceed.
func IsAvailable(ctx context.Context, client *redis.Client) bool {
	if client == nil {
		return false
	}
	pingCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return client.Ping(pingCtx).Err() == nil
}
