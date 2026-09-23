package config

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// ── optionalBoolWarn ──────────────────────────────────────────────────────────

func TestOptionalBoolWarn_Unparseable_ReturnsWarn(t *testing.T) {
	t.Setenv("REDIS_TLS", "notabool")

	val, warn := optionalBoolWarn("REDIS_TLS", false)
	if val {
		t.Error("expected optionalBoolWarn to return default false for unparseable value")
	}
	if warn == "" {
		t.Error("expected optionalBoolWarn to return a non-empty warning for unparseable value")
	}
	if !strings.Contains(warn, "REDIS_TLS") {
		t.Errorf("expected warning to mention the variable name, got: %q", warn)
	}
	if !strings.Contains(warn, "notabool") {
		t.Errorf("expected warning to mention the invalid value, got: %q", warn)
	}
}

func TestOptionalBoolWarn_Valid_NoWarn(t *testing.T) {
	t.Setenv("REDIS_TLS", "true")

	val, warn := optionalBoolWarn("REDIS_TLS", false)
	if !val {
		t.Error("expected optionalBoolWarn to return true for valid 'true' value")
	}
	if warn != "" {
		t.Errorf("expected no warning for valid value, got: %q", warn)
	}
}

func TestOptionalBoolWarn_Absent_NoWarn(t *testing.T) {
	// Ensure the env var is not set.
	t.Setenv("REDIS_TLS", "")

	val, warn := optionalBoolWarn("REDIS_TLS", true)
	if !val {
		t.Error("expected optionalBoolWarn to return default (true) when var is unset")
	}
	if warn != "" {
		t.Errorf("expected no warning when var is absent, got: %q", warn)
	}
}

// ── optionalDurationWarn ──────────────────────────────────────────────────────

func TestOptionalDurationWarn_Unparseable_ReturnsWarn(t *testing.T) {
	t.Setenv("SESSION_CLEANUP_INTERVAL", "foobar")

	val, warn := optionalDurationWarn("SESSION_CLEANUP_INTERVAL", time.Hour)
	if val != time.Hour {
		t.Errorf("expected default 1h for unparseable value, got: %v", val)
	}
	if warn == "" {
		t.Error("expected a non-empty warning for unparseable duration")
	}
	if !strings.Contains(warn, "SESSION_CLEANUP_INTERVAL") {
		t.Errorf("expected warning to mention the variable name, got: %q", warn)
	}
	if !strings.Contains(warn, "foobar") {
		t.Errorf("expected warning to mention the invalid value, got: %q", warn)
	}
}

func TestOptionalDurationWarn_Valid_NoWarn(t *testing.T) {
	t.Setenv("SESSION_CLEANUP_INTERVAL", "30m")

	val, warn := optionalDurationWarn("SESSION_CLEANUP_INTERVAL", time.Hour)
	if val != 30*time.Minute {
		t.Errorf("expected 30m for valid value, got: %v", val)
	}
	if warn != "" {
		t.Errorf("expected no warning for valid value, got: %q", warn)
	}
}

func TestOptionalDurationWarn_Absent_NoWarn(t *testing.T) {
	t.Setenv("SESSION_CLEANUP_INTERVAL", "")

	val, warn := optionalDurationWarn("SESSION_CLEANUP_INTERVAL", time.Hour)
	if val != time.Hour {
		t.Errorf("expected default 1h when var is absent, got: %v", val)
	}
	if warn != "" {
		t.Errorf("expected no warning when var is absent, got: %q", warn)
	}
}

// ── _FILE secret convention ───────────────────────────────────────────────────

func TestLoad_JWTSecretFile(t *testing.T) {
	// Write a secret file with a valid 32-byte secret.
	f, err := os.CreateTemp(t.TempDir(), "jwt_secret_*.txt")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	secret := "aaaabbbbccccddddeeeeffffgggghhhhiiiijjjj" // 40 bytes
	if _, err := f.WriteString(secret); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}

	vars := requiredVars()
	// Remove JWT_SECRET and set JWT_SECRET_FILE instead.
	delete(vars, "JWT_SECRET")
	vars["JWT_SECRET_FILE"] = f.Name()
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with JWT_SECRET_FILE: unexpected error: %v", err)
	}
	if string(cfg.JWTSecret) != secret {
		t.Errorf("JWTSecret: want %q, got %q", secret, string(cfg.JWTSecret))
	}
}

func TestLoad_JWTSecretFilePrecedence(t *testing.T) {
	// When both JWT_SECRET and JWT_SECRET_FILE are set, JWT_SECRET_FILE takes precedence.
	f, err := os.CreateTemp(t.TempDir(), "jwt_secret_*.txt")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	fileSecret := "file_secret_aaaabbbbccccddddeeeeffffgggghhhh" // 43 bytes
	if _, err := f.WriteString(fileSecret); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}

	vars := requiredVars()
	vars["JWT_SECRET_FILE"] = f.Name()
	// JWT_SECRET is also set (from requiredVars) — file should win.
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with both JWT_SECRET and JWT_SECRET_FILE: unexpected error: %v", err)
	}
	if string(cfg.JWTSecret) != fileSecret {
		t.Errorf("JWTSecret: want file value %q, got %q", fileSecret, string(cfg.JWTSecret))
	}
}

func TestLoad_JWTSecretFileMissing_FallsBackToEnv(t *testing.T) {
	// JWT_SECRET_FILE is not set; JWT_SECRET is used.
	vars := requiredVars()
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() without JWT_SECRET_FILE: unexpected error: %v", err)
	}
	if string(cfg.JWTSecret) != vars["JWT_SECRET"] {
		t.Errorf("JWTSecret: want env value %q, got %q", vars["JWT_SECRET"], string(cfg.JWTSecret))
	}
}

// ── Load with invalid optional vars produces warnings but succeeds ────────────

func TestLoad_InvalidOptionalBool_Succeeds(t *testing.T) {
	// An invalid REDIS_TLS value must not cause Load to fail.
	vars := requiredVars()
	vars["REDIS_TLS"] = "not-a-bool"
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with invalid REDIS_TLS: unexpected error: %v", err)
	}
	// Must use the default (false).
	if cfg.RedisTLS {
		t.Error("expected RedisTLS=false (default) for invalid REDIS_TLS value")
	}
}

func TestLoad_InvalidOptionalDuration_Succeeds(t *testing.T) {
	// An invalid SESSION_CLEANUP_INTERVAL value must not cause Load to fail.
	vars := requiredVars()
	vars["SESSION_CLEANUP_INTERVAL"] = "not-a-duration"
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with invalid SESSION_CLEANUP_INTERVAL: unexpected error: %v", err)
	}
	// Must use the default (1h).
	if cfg.SessionCleanupInterval != time.Hour {
		t.Errorf("expected SessionCleanupInterval=1h (default), got: %v", cfg.SessionCleanupInterval)
	}
}

func TestLoad_InvalidTitleWorkerInterval_Succeeds(t *testing.T) {
	// An invalid TITLE_WORKER_INTERVAL value must not cause Load to fail.
	// The default (5m) must be used instead.
	vars := requiredVars()
	vars["TITLE_WORKER_INTERVAL"] = "not-a-duration"
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with invalid TITLE_WORKER_INTERVAL: unexpected error: %v", err)
	}
	if cfg.TitleWorkerInterval != 5*time.Minute {
		t.Errorf("expected TitleWorkerInterval=5m (default), got: %v", cfg.TitleWorkerInterval)
	}
}

func TestLoad_InvalidTitleNotificationInterval_Succeeds(t *testing.T) {
	// An invalid TITLE_NOTIFICATION_INTERVAL value must not cause Load to fail.
	// The default (1m) must be used instead.
	vars := requiredVars()
	vars["TITLE_NOTIFICATION_INTERVAL"] = "not-a-duration"
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with invalid TITLE_NOTIFICATION_INTERVAL: unexpected error: %v", err)
	}
	if cfg.TitleNotificationInterval != time.Minute {
		t.Errorf("expected TitleNotificationInterval=1m (default), got: %v", cfg.TitleNotificationInterval)
	}
}

// ── stderr warning emission ───────────────────────────────────────────────────

// captureStderr redirects os.Stderr to a pipe while fn runs and returns everything
// written to it. fn runs synchronously and its output is read only after the write
// end is closed, so it must write less than the OS pipe buffer (config warnings are
// a few hundred bytes). Callers must not use t.Parallel(): os.Stderr is process-global.
// t.Cleanup restores os.Stderr even if fn aborts the test via t.Fatal.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = orig
		_ = w.Close() // no-op error if already closed below
		_ = r.Close()
	})

	fn()

	os.Stderr = orig
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return string(out)
}

func TestReadSecretFile_Unreadable_ReturnsEmptyAndWarns(t *testing.T) {
	// A path inside a fresh temp dir that is never created — guaranteed unreadable.
	t.Setenv("JWT_SECRET_FILE", filepath.Join(t.TempDir(), "missing_jwt_secret"))

	var got string
	stderr := captureStderr(t, func() {
		got = readSecretFile("JWT_SECRET")
	})

	if got != "" {
		t.Errorf("expected readSecretFile to return empty string for unreadable file, got: %q", got)
	}
	if !strings.Contains(stderr, "WARN config:") {
		t.Errorf("expected stderr to contain %q, got: %q", "WARN config:", stderr)
	}
	if !strings.Contains(stderr, "JWT_SECRET_FILE") {
		t.Errorf("expected stderr to mention JWT_SECRET_FILE, got: %q", stderr)
	}
}

func TestLoad_UnreadableSecretFile_FallsBackToEnv(t *testing.T) {
	// JWT_SECRET_FILE points at a missing file; Load must warn and fall back to JWT_SECRET.
	envSecret := "env_fallback_secret_aaaabbbbccccddddeeee" // 40 bytes, fake
	vars := requiredVars()
	vars["JWT_SECRET"] = envSecret
	vars["JWT_SECRET_FILE"] = filepath.Join(t.TempDir(), "missing_jwt_secret")
	setEnv(t, vars)

	var cfg *Config
	var err error
	stderr := captureStderr(t, func() {
		cfg, err = Load()
	})

	if err != nil {
		t.Fatalf("Load() with unreadable JWT_SECRET_FILE and valid JWT_SECRET: unexpected error: %v", err)
	}
	if string(cfg.JWTSecret) != envSecret {
		t.Error("JWTSecret: expected the JWT_SECRET environment value to be used as fallback")
	}
	if !strings.Contains(stderr, "WARN config:") || !strings.Contains(stderr, "JWT_SECRET_FILE") {
		t.Errorf("expected a JWT_SECRET_FILE warning on stderr, got: %q", stderr)
	}
	if strings.Contains(stderr, envSecret) {
		t.Error("stderr must not contain the JWT secret value")
	}
}

func TestLoad_UnreadableSecretFile_NoFallback_Fails(t *testing.T) {
	// JWT_SECRET_FILE is unreadable and JWT_SECRET is empty: no usable secret, Load must fail.
	vars := requiredVars()
	vars["JWT_SECRET"] = "" // empty is treated as unset by Load
	vars["JWT_SECRET_FILE"] = filepath.Join(t.TempDir(), "missing_jwt_secret")
	setEnv(t, vars)

	var err error
	stderr := captureStderr(t, func() {
		_, err = Load()
	})

	if err == nil {
		t.Fatal("expected Load() to fail when JWT_SECRET_FILE is unreadable and JWT_SECRET is unset")
	}
	if !strings.Contains(err.Error(), "JWT_SECRET_FILE") {
		t.Errorf("expected error to identify JWT_SECRET / JWT_SECRET_FILE, got: %v", err)
	}
	if !strings.Contains(stderr, "JWT_SECRET_FILE") {
		t.Errorf("expected a JWT_SECRET_FILE warning on stderr, got: %q", stderr)
	}
}

func TestLoad_InvalidOptional_EmitsStderrWarning(t *testing.T) {
	cases := []struct {
		key        string
		value      string
		isFallback func(*Config) bool
	}{
		{"REDIS_TLS", "not-a-bool", func(c *Config) bool { return !c.RedisTLS }},
		{"SESSION_CLEANUP_INTERVAL", "not-a-duration", func(c *Config) bool { return c.SessionCleanupInterval == time.Hour }},
		{"TITLE_WORKER_INTERVAL", "not-a-duration", func(c *Config) bool { return c.TitleWorkerInterval == 5*time.Minute }},
		{"TITLE_NOTIFICATION_INTERVAL", "not-a-duration", func(c *Config) bool { return c.TitleNotificationInterval == time.Minute }},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			vars := requiredVars()
			vars[tc.key] = tc.value
			setEnv(t, vars)

			var cfg *Config
			var err error
			stderr := captureStderr(t, func() {
				cfg, err = Load()
			})

			if err != nil {
				t.Fatalf("Load() with invalid %s: unexpected error: %v", tc.key, err)
			}
			if !tc.isFallback(cfg) {
				t.Errorf("expected default fallback value for invalid %s", tc.key)
			}
			if !strings.Contains(stderr, "WARN config:") {
				t.Errorf("expected stderr to contain %q, got: %q", "WARN config:", stderr)
			}
			if !strings.Contains(stderr, tc.key) {
				t.Errorf("expected stderr warning to mention %s, got: %q", tc.key, stderr)
			}
		})
	}
}
