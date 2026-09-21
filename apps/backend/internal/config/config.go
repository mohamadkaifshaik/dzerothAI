// Package config loads and validates application configuration from environment variables.
// Required variables are validated at startup; missing or invalid values cause a fast failure
// so the process never starts in a broken state.
//
// Secret file convention: for variables marked as secrets (JWT_SECRET, POSTGRES_PASSWORD,
// REDIS_PASSWORD), a companion <VAR>_FILE variant is supported. When <VAR>_FILE is set,
// the secret value is read from the file path it specifies. This follows the Docker Secrets
// standard pattern where secrets are mounted as files under /run/secrets/<name>.
//
// Precedence: if both <VAR>_FILE and <VAR> are set, <VAR>_FILE takes precedence.
// If neither is set, the variable is treated as missing (required) or empty (optional).
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
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
	// on the public API port. Default: "127.0.0.1:9091" (loopback only).
	// Use ":9091" only when a sidecar container (e.g. Prometheus) must reach the
	// admin port across the Docker bridge network.
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

	// SessionCleanupInterval controls how often the background worker removes
	// expired session rows from PostgreSQL. Accepts any Go duration string
	// (e.g. "1h", "30m"). Default: 1 hour.
	SessionCleanupInterval time.Duration

	// TitleWorkerInterval controls how often the title qualification worker runs
	// a full reconciliation pass over all users. Accepts any Go duration string
	// (e.g. "5m", "10m"). Default: 5 minutes.
	TitleWorkerInterval time.Duration

	// TitleNotificationInterval controls how often the title notification worker
	// dispatches pending title notifications. Accepts any Go duration string
	// (e.g. "1m", "5m"). Default: 1 minute.
	TitleNotificationInterval time.Duration
}

// Warnings collects non-fatal startup warnings about configuration issues.
// These are returned alongside the Config so the caller can log them.
var _ = (*Config)(nil) // compile-time check

// Load reads all required and optional environment variables, validates them, and
// returns a populated Config or an error describing what is missing or invalid.
// Callers should treat a non-nil error as fatal and exit the process.
//
// Warnings about invalid optional variables are printed to stderr. They do not
// cause Load to fail — the default value is used instead.
func Load() (*Config, error) {
	var errs []string
	var warns []string

	// requiredSecret reads a required secret value. It checks <KEY>_FILE first
	// (Docker Secrets convention), then falls back to <KEY>. If neither is set,
	// an error is recorded.
	requiredSecret := func(key string) string {
		if fileVar := readSecretFile(key); fileVar != "" {
			return fileVar
		}
		v := os.Getenv(key)
		if v == "" {
			errs = append(errs, fmt.Sprintf("required environment variable %s (or %s_FILE) is not set", key, key))
		}
		return v
	}

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
	pgPassword := requiredSecret("POSTGRES_PASSWORD")
	pgDB := required("POSTGRES_DB")
	pgHost := required("POSTGRES_HOST")
	redisAddr := required("REDIS_ADDR")
	jwtSecretStr := requiredSecret("JWT_SECRET")

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

	// Optional secret: REDIS_PASSWORD supports _FILE convention.
	redisPassword := ""
	if fileVal := readSecretFile("REDIS_PASSWORD"); fileVal != "" {
		redisPassword = fileVal
	} else {
		redisPassword = optional("REDIS_PASSWORD", "")
	}

	// Optional bool: warn if set but invalid.
	redisTLS, warn := optionalBoolWarn("REDIS_TLS", false)
	if warn != "" {
		warns = append(warns, warn)
	}

	// Optional duration: warn if set but invalid.
	sessionInterval, warn := optionalDurationWarn("SESSION_CLEANUP_INTERVAL", time.Hour)
	if warn != "" {
		warns = append(warns, warn)
	}

	// Optional duration: warn if set but invalid. Default: 5 minutes.
	titleWorkerInterval, warn := optionalDurationWarn("TITLE_WORKER_INTERVAL", 5*time.Minute)
	if warn != "" {
		warns = append(warns, warn)
	}

	// Optional duration: warn if set but invalid. Default: 1 minute.
	titleNotificationInterval, warn := optionalDurationWarn("TITLE_NOTIFICATION_INTERVAL", 1*time.Minute)
	if warn != "" {
		warns = append(warns, warn)
	}

	// Print startup warnings to stderr. These are non-fatal; the default is used.
	for _, w := range warns {
		_, _ = fmt.Fprintf(os.Stderr, "WARN config: %s\n", w)
	}

	return &Config{
		PostgresDSN:               dsn,
		RedisAddr:                 redisAddr,
		JWTSecret:                 jwtSecret,
		APIPort:                   optional("API_PORT", "8080"),
		Environment:               optional("ENVIRONMENT", "local"),
		LogLevel:                  optional("LOG_LEVEL", "info"),
		CORSAllowedOrigins:        corsOrigins,
		AdminAddr:                 optional("ADMIN_ADDR", "127.0.0.1:9091"),
		PostgresSSLMode:           pgSSLMode,
		RedisPassword:             redisPassword,
		RedisTLS:                  redisTLS,
		SessionCleanupInterval:    sessionInterval,
		TitleWorkerInterval:       titleWorkerInterval,
		TitleNotificationInterval: titleNotificationInterval,
	}, nil
}

// readSecretFile checks whether <KEY>_FILE is set and, if so, reads and returns
// the trimmed file contents. Returns "" if the env var is unset or the file is empty.
// Returns "" (with a stderr warning) if the file cannot be read.
func readSecretFile(key string) string {
	fileKey := key + "_FILE"
	path := os.Getenv(fileKey)
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "WARN config: %s is set but file %q cannot be read: %v\n", fileKey, path, err)
		return ""
	}
	return strings.TrimRight(string(data), "\r\n")
}

// optionalBool reads a boolean environment variable. When the variable is unset or
// empty the default is returned. Unparseable values are silently treated as the default
// (consistent with the pass-through philosophy used for other optional fields).
// Prefer optionalBoolWarn for new callers — it surfaces the invalid value as a warning.
func optionalBool(key string, def bool) bool {
	v, _ := optionalBoolWarn(key, def)
	return v
}

// optionalBoolWarn reads a boolean environment variable and returns the parsed value
// plus a non-empty warning string if the variable is set but unparseable.
// When the variable is unset or empty, the default is returned with no warning.
func optionalBoolWarn(key string, def bool) (bool, string) {
	v := os.Getenv(key)
	if v == "" {
		return def, ""
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def, fmt.Sprintf("%s is set to %q which is not a valid boolean; using default %v", key, v, def)
	}
	return b, ""
}

// optionalDuration reads a Go duration environment variable. When the variable is
// unset, empty, or unparseable the default is returned.
// Prefer optionalDurationWarn for new callers — it surfaces the invalid value as a warning.
func optionalDuration(key string, def time.Duration) time.Duration {
	v, _ := optionalDurationWarn(key, def)
	return v
}

// optionalDurationWarn reads a Go duration environment variable and returns the parsed
// value plus a non-empty warning string if the variable is set but unparseable.
// When the variable is unset or empty, the default is returned with no warning.
func optionalDurationWarn(key string, def time.Duration) (time.Duration, string) {
	v := os.Getenv(key)
	if v == "" {
		return def, ""
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def, fmt.Sprintf("%s is set to %q which is not a valid Go duration; using default %s", key, v, def)
	}
	return d, ""
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
