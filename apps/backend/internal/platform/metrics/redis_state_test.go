package metrics_test

// redis_state_test.go covers the Redis availability state-change structured logging
// behaviour of InfraMetrics.SetRedisUp. It uses go.uber.org/zap/zaptest/observer to
// capture log output in-process without writing to disk.

import (
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/metrics"
)

// newObservedInfra creates an InfraMetrics instance backed by a fresh isolated registry
// and an observed zap logger. The observer allows tests to inspect log entries produced
// by state-change transitions without writing to any I/O sink.
func newObservedInfra(t *testing.T) (*metrics.InfraMetrics, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log := zap.New(core)
	reg := prometheus.NewRegistry()
	im := metrics.NewInfraMetrics(reg, log)
	return im, logs
}

// stateChangeLogs returns only the entries from logs whose message is
// "redis availability changed". This isolates state-change entries from any
// other log messages that might be added in the future.
func stateChangeLogs(logs *observer.ObservedLogs) []observer.LoggedEntry {
	var out []observer.LoggedEntry
	for _, e := range logs.All() {
		if e.Message == "redis availability changed" {
			out = append(out, e)
		}
	}
	return out
}

// fieldBool returns the bool value of the named field from a LoggedEntry, and whether
// the field was found.
func fieldBool(e observer.LoggedEntry, key string) (bool, bool) {
	for _, f := range e.Context {
		if f.Key == key && f.Type == zapcore.BoolType {
			return f.Integer == 1, true
		}
	}
	return false, false
}

// fieldString returns the string value of the named field from a LoggedEntry, and
// whether the field was found.
func fieldString(e observer.LoggedEntry, key string) (string, bool) {
	for _, f := range e.Context {
		if f.Key == key && f.Type == zapcore.StringType {
			return f.String, true
		}
	}
	return "", false
}

// TestRedisStateChange_InitialState_NoLog verifies that the very first call to
// SetRedisUp does not produce a state-change log entry, because there is no
// previous state to transition from.
func TestRedisStateChange_InitialState_NoLog(t *testing.T) {
	im, logs := newObservedInfra(t)

	im.SetRedisUp(true)

	entries := stateChangeLogs(logs)
	if len(entries) != 0 {
		t.Errorf("initial SetRedisUp(true): want 0 state-change log entries, got %d", len(entries))
	}
}

// TestRedisStateChange_AvailableToUnavailable_Warn verifies that a transition from
// available to unavailable produces exactly one WARN log entry with the correct fields.
func TestRedisStateChange_AvailableToUnavailable_Warn(t *testing.T) {
	im, logs := newObservedInfra(t)

	im.SetRedisUp(true)  // initial state — no log
	im.SetRedisUp(false) // transition → should produce WARN

	entries := stateChangeLogs(logs)
	if len(entries) != 1 {
		t.Fatalf("available→unavailable: want 1 state-change log entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Level != zapcore.WarnLevel {
		t.Errorf("available→unavailable: want WARN level, got %s", e.Level)
	}

	if prev, ok := fieldBool(e, "previous_available"); !ok || !prev {
		t.Errorf("available→unavailable: want previous_available=true, got ok=%v val=%v", ok, prev)
	}
	if avail, ok := fieldBool(e, "available"); !ok || avail {
		t.Errorf("available→unavailable: want available=false, got ok=%v val=%v", ok, avail)
	}
	if comp, ok := fieldString(e, "component"); !ok || comp != "redis" {
		t.Errorf("available→unavailable: want component=redis, got ok=%v val=%q", ok, comp)
	}
}

// TestRedisStateChange_UnavailableToAvailable_Info verifies that a transition from
// unavailable to available produces exactly one INFO log entry with the correct fields.
func TestRedisStateChange_UnavailableToAvailable_Info(t *testing.T) {
	im, logs := newObservedInfra(t)

	im.SetRedisUp(false) // initial state — no log
	im.SetRedisUp(true)  // transition → should produce INFO

	entries := stateChangeLogs(logs)
	if len(entries) != 1 {
		t.Fatalf("unavailable→available: want 1 state-change log entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Level != zapcore.InfoLevel {
		t.Errorf("unavailable→available: want INFO level, got %s", e.Level)
	}

	if prev, ok := fieldBool(e, "previous_available"); !ok || prev {
		t.Errorf("unavailable→available: want previous_available=false, got ok=%v val=%v", ok, prev)
	}
	if avail, ok := fieldBool(e, "available"); !ok || !avail {
		t.Errorf("unavailable→available: want available=true, got ok=%v val=%v", ok, avail)
	}
	if comp, ok := fieldString(e, "component"); !ok || comp != "redis" {
		t.Errorf("unavailable→available: want component=redis, got ok=%v val=%q", ok, comp)
	}
}

// TestRedisStateChange_RepeatUnavailable_NoLog verifies that two consecutive calls
// to SetRedisUp(false) produce only the initial state recording (no log) — the second
// call with the same state must not produce a log entry.
func TestRedisStateChange_RepeatUnavailable_NoLog(t *testing.T) {
	im, logs := newObservedInfra(t)

	im.SetRedisUp(false) // initial — no log
	im.SetRedisUp(false) // same state — no log

	entries := stateChangeLogs(logs)
	if len(entries) != 0 {
		t.Errorf("repeated SetRedisUp(false): want 0 state-change log entries, got %d", len(entries))
	}
}

// TestRedisStateChange_RepeatAvailable_NoLog verifies that two consecutive calls
// to SetRedisUp(true) produce only the initial state recording (no log) — the second
// call with the same state must not produce a log entry.
func TestRedisStateChange_RepeatAvailable_NoLog(t *testing.T) {
	im, logs := newObservedInfra(t)

	im.SetRedisUp(true) // initial — no log
	im.SetRedisUp(true) // same state — no log

	entries := stateChangeLogs(logs)
	if len(entries) != 0 {
		t.Errorf("repeated SetRedisUp(true): want 0 state-change log entries, got %d", len(entries))
	}
}

// TestRedisStateChange_Concurrent verifies that concurrent calls to SetRedisUp do not
// cause data races. This test is designed to be run with the race detector enabled
// (go test -race). It does not assert on specific log counts because the interleaving
// is nondeterministic; it only verifies the absence of panics and races.
func TestRedisStateChange_Concurrent(t *testing.T) {
	im, _ := newObservedInfra(t)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			im.SetRedisUp(i%2 == 0)
		}(i)
	}
	wg.Wait()
	// No panic or race detected by the race detector — test passes.
}

// TestRedisStateChange_NoSensitiveData verifies that state-change log entries do not
// contain connection strings, Redis key names, passwords, or other sensitive values.
// Only "previous_available", "available", and "component" fields are expected.
func TestRedisStateChange_NoSensitiveData(t *testing.T) {
	im, logs := newObservedInfra(t)

	im.SetRedisUp(true)
	im.SetRedisUp(false) // produces a log entry

	entries := stateChangeLogs(logs)
	if len(entries) == 0 {
		t.Fatal("expected at least one state-change log entry")
	}

	sensitivePatterns := []string{
		"redis://", "rediss://", "password", "secret", "token",
		"addr", "host", "key", "dsn",
	}

	for _, e := range entries {
		for _, f := range e.Context {
			for _, pattern := range sensitivePatterns {
				if strings.Contains(strings.ToLower(f.Key), pattern) {
					t.Errorf("state-change log: field key %q matches sensitive pattern %q", f.Key, pattern)
				}
				if strings.Contains(strings.ToLower(f.String), pattern) {
					t.Errorf("state-change log: field %q value %q matches sensitive pattern %q", f.Key, f.String, pattern)
				}
			}
		}
	}
}
