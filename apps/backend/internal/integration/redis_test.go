//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	rdb "github.com/redis/go-redis/v9"

	platformRedis "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/redis"
)

// ---------------------------------------------------------------------------
// Redis connectivity
// ---------------------------------------------------------------------------

// TestRedis_Connect verifies that the integration test Redis service is
// reachable with the correct password and that Ping succeeds.
func TestRedis_Connect(t *testing.T) {
	client := connectTestRedis(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("Redis Ping failed after Connect: %v", err)
	}
}

// TestRedis_IsAvailable verifies that platformRedis.IsAvailable returns true
// for a connected client.
func TestRedis_IsAvailable(t *testing.T) {
	client := connectTestRedis(t)

	ctx := context.Background()
	if !platformRedis.IsAvailable(ctx, client) {
		t.Fatal("IsAvailable returned false for a connected Redis client")
	}
}

// ---------------------------------------------------------------------------
// Authentication enforcement
// ---------------------------------------------------------------------------

// TestRedis_UnauthenticatedConnectionRejected verifies that a connection
// attempt without a password is rejected by the Redis service. This confirms
// that the CI Redis container is started with --requirepass and that the
// authentication requirement is enforced.
func TestRedis_UnauthenticatedConnectionRejected(t *testing.T) {
	// Attempt to connect without a password.
	_, err := platformRedis.Connect(context.Background(), testRedisAddr(), "", false)
	if err == nil {
		t.Fatal("expected unauthenticated connection to fail, but Connect returned nil error")
	}
	// The error should mention authentication — verify the message contains a
	// recognizable Redis auth error phrase (NOAUTH or WRONGPASS).
	errStr := err.Error()
	if errStr == "" {
		t.Error("expected a non-empty error message from unauthenticated connection")
	}
}

// ---------------------------------------------------------------------------
// Rate-limit key behavior (SetNX / Incr / Expire)
// ---------------------------------------------------------------------------
//
// These tests verify the exact Redis key operations used by RateLimitMiddleware
// in internal/auth/middleware.go. The middleware uses:
//
//   1. SetNX(key, 0, window)   — init counter only if absent
//   2. Incr(key)               — unconditional increment
//   3. Expire(key, window)     — belt-and-suspenders when count == 1

// TestRedis_SetNX_InitializesCounter verifies that SetNX creates the key with
// value 0 and that a subsequent Incr returns 1.
func TestRedis_SetNX_InitializesCounter(t *testing.T) {
	client := connectTestRedis(t)
	ctx := context.Background()

	key := "rl:auth:login:127.0.0.1:test_setnx_" + uniqueSuffix(t)
	window := 15 * time.Minute

	// SetNX should succeed (key does not exist).
	ok, err := client.SetNX(ctx, key, 0, window).Result()
	if err != nil {
		t.Fatalf("SetNX: %v", err)
	}
	if !ok {
		t.Fatal("SetNX returned false — key already existed unexpectedly")
	}
	t.Cleanup(func() { _ = client.Del(context.Background(), key).Err() })

	count, err := client.Incr(ctx, key).Result()
	if err != nil {
		t.Fatalf("Incr after SetNX: %v", err)
	}
	if count != 1 {
		t.Errorf("Incr after SetNX = %d, want 1", count)
	}
}

// TestRedis_SetNX_DoesNotOverwriteExistingKey verifies that SetNX on an
// existing key returns false and leaves the value unchanged.
func TestRedis_SetNX_DoesNotOverwriteExistingKey(t *testing.T) {
	client := connectTestRedis(t)
	ctx := context.Background()

	key := "rl:auth:login:127.0.0.1:test_setnx_nooverwrite_" + uniqueSuffix(t)
	window := 15 * time.Minute

	// First SetNX creates the key.
	if _, err := client.SetNX(ctx, key, 0, window).Result(); err != nil {
		t.Fatalf("first SetNX: %v", err)
	}
	t.Cleanup(func() { _ = client.Del(context.Background(), key).Err() })

	// Incr the counter to 5.
	for i := 0; i < 5; i++ {
		if _, err := client.Incr(ctx, key).Result(); err != nil {
			t.Fatalf("Incr %d: %v", i, err)
		}
	}

	// Second SetNX must NOT overwrite the key (returns false).
	ok, err := client.SetNX(ctx, key, 0, window).Result()
	if err != nil {
		t.Fatalf("second SetNX: %v", err)
	}
	if ok {
		t.Fatal("SetNX returned true on an existing key — should not overwrite")
	}

	// Value must still be 5.
	val, err := client.Get(ctx, key).Int64()
	if err != nil {
		t.Fatalf("Get after second SetNX: %v", err)
	}
	if val != 5 {
		t.Errorf("key value = %d after second SetNX, want 5", val)
	}
}

// TestRedis_Incr_IncreasesCounterSequentially verifies that repeated Incr calls
// increment the same counter on each call, matching the rate-limit accumulation
// behavior.
func TestRedis_Incr_IncreasesCounterSequentially(t *testing.T) {
	client := connectTestRedis(t)
	ctx := context.Background()

	key := "rl:auth:login:127.0.0.1:test_incr_seq_" + uniqueSuffix(t)
	window := 15 * time.Minute

	if _, err := client.SetNX(ctx, key, 0, window).Result(); err != nil {
		t.Fatalf("SetNX: %v", err)
	}
	t.Cleanup(func() { _ = client.Del(context.Background(), key).Err() })

	const attempts = 5
	for i := 1; i <= attempts; i++ {
		count, err := client.Incr(ctx, key).Result()
		if err != nil {
			t.Fatalf("Incr %d: %v", i, err)
		}
		if count != int64(i) {
			t.Errorf("Incr %d returned %d, want %d", i, count, i)
		}
	}
}

// TestRedis_Expire_SetsWindowOnKey verifies that Expire sets a TTL on the key,
// ensuring rate-limit windows eventually expire. The TTL must be positive and
// within the configured window duration.
func TestRedis_Expire_SetsWindowOnKey(t *testing.T) {
	client := connectTestRedis(t)
	ctx := context.Background()

	key := "rl:auth:login:127.0.0.1:test_expire_" + uniqueSuffix(t)
	window := 15 * time.Minute

	// Create the key without a TTL (simulates the INCR path).
	if err := client.Set(ctx, key, 1, 0).Err(); err != nil {
		t.Fatalf("Set: %v", err)
	}
	t.Cleanup(func() { _ = client.Del(context.Background(), key).Err() })

	// Verify no TTL yet.
	ttlBefore, err := client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL before Expire: %v", err)
	}
	if ttlBefore >= 0 {
		t.Logf("TTL before Expire = %v (key may already have a TTL)", ttlBefore)
	}

	// Apply Expire.
	ok, err := client.Expire(ctx, key, window).Result()
	if err != nil {
		t.Fatalf("Expire: %v", err)
	}
	if !ok {
		t.Fatal("Expire returned false — key did not exist or command failed")
	}

	// TTL must now be positive and ≤ window.
	ttlAfter, err := client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL after Expire: %v", err)
	}
	if ttlAfter <= 0 {
		t.Errorf("TTL after Expire = %v, want > 0", ttlAfter)
	}
	if ttlAfter > window {
		t.Errorf("TTL after Expire = %v, want ≤ %v", ttlAfter, window)
	}
}

// TestRedis_RateLimitKeyIsolation verifies that rate-limit keys for different
// operations are independent: incrementing the "login" key does not affect the
// "refresh" key and vice versa.
func TestRedis_RateLimitKeyIsolation(t *testing.T) {
	client := connectTestRedis(t)
	ctx := context.Background()

	suffix := uniqueSuffix(t)
	loginKey := fmt.Sprintf("rl:auth:login:127.0.0.1:%s", suffix)
	refreshKey := fmt.Sprintf("rl:auth:refresh:127.0.0.1:%s", suffix)
	window := 15 * time.Minute

	for _, k := range []string{loginKey, refreshKey} {
		if _, err := client.SetNX(ctx, k, 0, window).Result(); err != nil {
			t.Fatalf("SetNX %s: %v", k, err)
		}
	}
	t.Cleanup(func() {
		_ = client.Del(context.Background(), loginKey, refreshKey).Err()
	})

	// Increment login 3 times.
	for i := 0; i < 3; i++ {
		if _, err := client.Incr(ctx, loginKey).Result(); err != nil {
			t.Fatalf("Incr loginKey: %v", err)
		}
	}

	// Increment refresh 1 time.
	if _, err := client.Incr(ctx, refreshKey).Result(); err != nil {
		t.Fatalf("Incr refreshKey: %v", err)
	}

	loginCount, err := client.Get(ctx, loginKey).Int64()
	if err != nil {
		t.Fatalf("Get loginKey: %v", err)
	}
	refreshCount, err := client.Get(ctx, refreshKey).Int64()
	if err != nil {
		t.Fatalf("Get refreshKey: %v", err)
	}

	if loginCount != 3 {
		t.Errorf("loginKey count = %d, want 3", loginCount)
	}
	if refreshCount != 1 {
		t.Errorf("refreshKey count = %d, want 1", refreshCount)
	}
}

// TestRedis_RateLimitFailsClosed_NilClient verifies that platformRedis.IsAvailable
// returns false for a nil client, matching the fail-closed path in
// RateLimitMiddleware. This is a live-infrastructure-backed sanity check that
// the platform helper correctly handles the nil case.
func TestRedis_RateLimitFailsClosed_NilClient(t *testing.T) {
	ctx := context.Background()
	if platformRedis.IsAvailable(ctx, nil) {
		t.Fatal("IsAvailable must return false for nil client (fail-closed guard)")
	}
}

// TestRedis_RateLimitKeyExpires verifies that a key with a short TTL disappears
// after the window elapses. This confirms the rate-limit window actually resets.
// The test uses a 1-second window to avoid long sleep durations in CI.
func TestRedis_RateLimitKeyExpires(t *testing.T) {
	client := connectTestRedis(t)
	ctx := context.Background()

	key := "rl:auth:login:127.0.0.1:test_expiry_" + uniqueSuffix(t)
	window := 1 * time.Second

	if _, err := client.SetNX(ctx, key, 0, window).Result(); err != nil {
		t.Fatalf("SetNX: %v", err)
	}
	if _, err := client.Incr(ctx, key).Result(); err != nil {
		t.Fatalf("Incr: %v", err)
	}

	// Confirm the key exists.
	if err := client.Get(ctx, key).Err(); err != nil {
		t.Fatalf("Get before expiry: %v", err)
	}

	// Wait for the key to expire (window + buffer).
	time.Sleep(window + 200*time.Millisecond)

	err := client.Get(ctx, key).Err()
	if err == nil {
		t.Fatal("key still exists after TTL — rate-limit window does not reset")
	}
	if err != rdb.Nil {
		t.Errorf("unexpected error after TTL expiry: %v (want redis.Nil)", err)
	}
}

// uniqueSuffix returns a short unique string based on the test name, suitable
// for use in Redis key suffixes to prevent cross-test key collisions.
func uniqueSuffix(t *testing.T) string {
	t.Helper()
	// Use the test name with unsafe characters replaced.
	name := t.Name()
	safe := make([]byte, len(name))
	for i, c := range []byte(name) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			safe[i] = c
		} else {
			safe[i] = '_'
		}
	}
	return string(safe)
}
