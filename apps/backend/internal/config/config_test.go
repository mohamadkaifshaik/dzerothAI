package config

import (
	"strings"
	"testing"
)

// setEnv sets a map of environment variables for the duration of a test and restores
// the previous values (or unsets them) in the cleanup function.
func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		prev, existed := lookupEnv(k)
		t.Setenv(k, v)
		// t.Setenv automatically calls os.Unsetenv or os.Setenv(prev) on cleanup
		// only when the key was already in the environment. For variables that were
		// not present before the test we need Unsetenv — t.Setenv handles this
		// correctly in Go 1.17+.
		_ = prev
		_ = existed
	}
}

// requiredVars returns the minimum set of env vars needed for config.Load() to succeed.
func requiredVars() map[string]string {
	return map[string]string{
		"POSTGRES_USER":     "testuser",
		"POSTGRES_PASSWORD": "testpass",
		"POSTGRES_DB":       "testdb",
		"POSTGRES_HOST":     "localhost",
		"REDIS_ADDR":        "localhost:6379",
		// JWT_SECRET must be >= 32 bytes
		"JWT_SECRET": "aaaabbbbccccddddeeeeffffgggghhhhiiiijjjj",
	}
}

// lookupEnv is a thin wrapper kept here so the file compiles without importing os
// directly (t.Setenv already handles os calls; this is only used for documentation).
func lookupEnv(_ string) (string, bool) { return "", false }

// ── PostgreSQL SSL Mode ───────────────────────────────────────────────────────

func TestConfig_PostgresSSLMode_Default(t *testing.T) {
	vars := requiredVars()
	// POSTGRES_SSL_MODE intentionally not set — should default to "disable".
	delete(vars, "POSTGRES_SSL_MODE")
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !strings.Contains(cfg.PostgresDSN, "sslmode=disable") {
		t.Errorf("expected DSN to contain sslmode=disable, got: %s", cfg.PostgresDSN)
	}
	if cfg.PostgresSSLMode != "disable" {
		t.Errorf("expected PostgresSSLMode=disable, got: %s", cfg.PostgresSSLMode)
	}
}

func TestConfig_PostgresSSLMode_Require(t *testing.T) {
	vars := requiredVars()
	vars["POSTGRES_SSL_MODE"] = "require"
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !strings.Contains(cfg.PostgresDSN, "sslmode=require") {
		t.Errorf("expected DSN to contain sslmode=require, got: %s", cfg.PostgresDSN)
	}
	if cfg.PostgresSSLMode != "require" {
		t.Errorf("expected PostgresSSLMode=require, got: %s", cfg.PostgresSSLMode)
	}
}

func TestConfig_PostgresSSLMode_VerifyFull(t *testing.T) {
	vars := requiredVars()
	vars["POSTGRES_SSL_MODE"] = "verify-full"
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !strings.Contains(cfg.PostgresDSN, "sslmode=verify-full") {
		t.Errorf("expected DSN to contain sslmode=verify-full, got: %s", cfg.PostgresDSN)
	}
	if cfg.PostgresSSLMode != "verify-full" {
		t.Errorf("expected PostgresSSLMode=verify-full, got: %s", cfg.PostgresSSLMode)
	}
}

// The existing config philosophy is pass-through for optional string fields.
// An invalid sslmode value is accepted by config.Load(); the pgx driver rejects it
// at connect time. This test documents the pass-through behavior.
func TestConfig_PostgresSSLMode_InvalidPassThrough(t *testing.T) {
	vars := requiredVars()
	vars["POSTGRES_SSL_MODE"] = "bogus-mode"
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v (unexpected — config uses pass-through philosophy)", err)
	}

	if !strings.Contains(cfg.PostgresDSN, "sslmode=bogus-mode") {
		t.Errorf("expected DSN to pass through sslmode=bogus-mode, got: %s", cfg.PostgresDSN)
	}
}

// ── Redis Password ────────────────────────────────────────────────────────────

func TestConfig_RedisPassword_Default(t *testing.T) {
	vars := requiredVars()
	// REDIS_PASSWORD intentionally not set — should default to empty string.
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.RedisPassword != "" {
		t.Errorf("expected RedisPassword to be empty by default, got: %q", cfg.RedisPassword)
	}
}

func TestConfig_RedisPassword_Set(t *testing.T) {
	vars := requiredVars()
	vars["REDIS_PASSWORD"] = "supersecret"
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.RedisPassword != "supersecret" {
		t.Errorf("expected RedisPassword=supersecret, got: %q", cfg.RedisPassword)
	}
}

// ── Redis TLS ─────────────────────────────────────────────────────────────────

func TestConfig_RedisTLS_Default(t *testing.T) {
	vars := requiredVars()
	// REDIS_TLS intentionally not set — should default to false.
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.RedisTLS {
		t.Error("expected RedisTLS=false by default, got true")
	}
}

func TestConfig_RedisTLS_True(t *testing.T) {
	vars := requiredVars()
	vars["REDIS_TLS"] = "true"
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.RedisTLS {
		t.Error("expected RedisTLS=true, got false")
	}
}

func TestConfig_RedisTLS_False(t *testing.T) {
	vars := requiredVars()
	vars["REDIS_TLS"] = "false"
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.RedisTLS {
		t.Error("expected RedisTLS=false, got true")
	}
}

// optionalBool silently returns the default for unparseable values.
func TestOptionalBool_Unparseable(t *testing.T) {
	t.Setenv("REDIS_TLS", "notabool")

	result := optionalBool("REDIS_TLS", false)
	if result {
		t.Error("expected optionalBool to return false for unparseable value")
	}
}

func TestOptionalBool_Empty(t *testing.T) {
	t.Setenv("REDIS_TLS", "")

	result := optionalBool("REDIS_TLS", true)
	if !result {
		t.Error("expected optionalBool to return default (true) for empty value")
	}
}
