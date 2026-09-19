// Package config loads and validates application configuration from environment variables.
// Required variables are validated at startup; missing or invalid values cause a fast failure
// so the process never starts in a broken state.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration resolved from environment variables.
type Config struct {
	// PostgresDSN is constructed from the POSTGRES_* environment variables.
	PostgresDSN string

	// RedisAddr is the Redis server address (host:port).
	RedisAddr string

	// JWTSecret is the raw signing secret. Minimum 32 bytes is enforced at Load time.
	JWTSecret []byte

	// APIPort is the TCP port the HTTP server listens on. Default: "8080".
	APIPort string

	// Environment is one of: local, test, staging, production. Default: "local".
	Environment string

	// LogLevel is one of: debug, info, warn, error. Default: "info".
	LogLevel string

	// CORSAllowedOrigins is the list of exact origins permitted in non-local/test
	// environments. Parsed from CORS_ALLOWED_ORIGINS (comma-separated). An empty
	// slice means no cross-origin requests are permitted — the server still starts.
	CORSAllowedOrigins []string

	// AdminAddr is the TCP listen address for the admin/observability HTTP server.
	// This server exposes /livez, /readyz, and /metrics — it must NOT be exposed
	// on the public API port. Default: ":9091".
	AdminAddr string

	// PostgresSSLMode is the sslmode query parameter passed to the PostgreSQL DSN.
	// Valid values: disable, allow, prefer, require, verify-ca, verify-full.
	// Invalid values are passed through and rejected by the pgx driver at connect time.
	// Default: "disable".
	PostgresSSLMode string

	// RedisPassword is the AUTH password sent to Redis on connect.
	// Leave empty for unauthenticated local Redis.
	RedisPassword string

	// RedisTLS enables TLS for the Redis connection when true.
	// Uses the system's default CA certificates — no self-signed support without additional config.
	// Default: false.
	RedisTLS bool
}

// Load reads all required and optional environment variables, validates them, and
// returns a populated Config or an error describing what is missing or invalid.
// Callers should treat a non-nil error as fatal and exit the process.
func Load() (*Config, error) {
	var errs []string

	required := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			errs = append(errs, fmt.Sprintf("required environment variable %s is not set", key))
		}
		return v
	}

	optional := func(key, defaultValue string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return defaultValue
	}

	pgUser := required("POSTGRES_USER")
	pgPassword := required("POSTGRES_PASSWORD")
	pgDB := required("POSTGRES_DB")
	pgHost := required("POSTGRES_HOST")
	redisAddr := required("REDIS_ADDR")
	jwtSecretStr := required("JWT_SECRET")

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}

	jwtSecret := []byte(jwtSecretStr)
	if len(jwtSecret) < 32 {
		return nil, fmt.Errorf(
			"JWT_SECRET must be at least 32 bytes; got %d bytes — increase the secret length",
			len(jwtSecret),
		)
	}

	pgPort := optional("POSTGRES_PORT", "5432")
	pgSSLMode := optional("POSTGRES_SSL_MODE", "disable")

	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		pgUser, pgPassword, pgHost, pgPort, pgDB, pgSSLMode,
	)

	corsOrigins := parseCORSOrigins(os.Getenv("CORS_ALLOWED_ORIGINS"))

	return &Config{
		PostgresDSN:        dsn,
		RedisAddr:          redisAddr,
		JWTSecret:          jwtSecret,
		APIPort:            optional("API_PORT", "8080"),
		Environment:        optional("ENVIRONMENT", "local"),
		LogLevel:           optional("LOG_LEVEL", "info"),
		CORSAllowedOrigins: corsOrigins,
		AdminAddr:          optional("ADMIN_ADDR", ":9091"),
		PostgresSSLMode:    pgSSLMode,
		RedisPassword:      optional("REDIS_PASSWORD", ""),
		RedisTLS:           optionalBool("REDIS_TLS", false),
	}, nil
}

// optionalBool reads a boolean environment variable. When the variable is unset or
// empty the default is returned. Unparseable values are silently treated as the default
// (consistent with the pass-through philosophy used for other optional fields).
func optionalBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// parseCORSOrigins splits a comma-separated origin list, trims whitespace, and
// filters empty strings. Returns an empty (non-nil) slice when the input is blank.
func parseCORSOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
