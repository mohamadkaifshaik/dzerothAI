package redis

import (
	"testing"
)

// These tests verify option construction without requiring a live Redis server.
// They use the internal buildOptions helper to inspect the redis.Options that
// Connect() would pass to go-redis.

// ── Password ──────────────────────────────────────────────────────────────────

func TestBuildOptions_NoPassword(t *testing.T) {
	opts := buildOptions("localhost:6379", "", false)
	if opts.Password != "" {
		t.Errorf("expected empty password, got %q", opts.Password)
	}
}

func TestBuildOptions_WithPassword(t *testing.T) {
	opts := buildOptions("localhost:6379", "mysecret", false)
	if opts.Password != "mysecret" {
		t.Errorf("expected password=mysecret, got %q", opts.Password)
	}
}

// ── TLS ───────────────────────────────────────────────────────────────────────

func TestBuildOptions_TLSDisabled(t *testing.T) {
	opts := buildOptions("localhost:6379", "", false)
	if opts.TLSConfig != nil {
		t.Error("expected TLSConfig to be nil when useTLS=false")
	}
}

func TestBuildOptions_TLSEnabled(t *testing.T) {
	opts := buildOptions("localhost:6379", "", true)
	if opts.TLSConfig == nil {
		t.Error("expected TLSConfig to be non-nil when useTLS=true")
	}
}

// ── Addr ──────────────────────────────────────────────────────────────────────

func TestBuildOptions_AddrIsSet(t *testing.T) {
	opts := buildOptions("redis.example.com:6380", "", false)
	if opts.Addr != "redis.example.com:6380" {
		t.Errorf("expected Addr=redis.example.com:6380, got %q", opts.Addr)
	}
}

// ── Security: password must not be observable through Addr field ──────────────

// TestBuildOptions_PasswordNotInAddr verifies that the password never appears in
// the Addr field (which is logged in error messages from Connect).
func TestBuildOptions_PasswordNotInAddr(t *testing.T) {
	const password = "should-not-leak"
	opts := buildOptions("localhost:6379", password, false)
	if opts.Addr == password || len(opts.Addr) > len("localhost:6379") {
		t.Errorf("password must not appear in Addr field; Addr=%q", opts.Addr)
	}
}

// ── Combined options ──────────────────────────────────────────────────────────

func TestBuildOptions_PasswordAndTLS(t *testing.T) {
	opts := buildOptions("secure.redis.example.com:6380", "prodpassword", true)
	if opts.Password != "prodpassword" {
		t.Errorf("expected password=prodpassword, got %q", opts.Password)
	}
	if opts.TLSConfig == nil {
		t.Error("expected TLSConfig to be non-nil")
	}
	if opts.Addr != "secure.redis.example.com:6380" {
		t.Errorf("expected correct Addr, got %q", opts.Addr)
	}
}
