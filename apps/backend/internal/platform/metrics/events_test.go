package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	ioprometheusclient "github.com/prometheus/client_model/go"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/metrics"
)

// newTestEvents creates an Events instance backed by a fresh isolated registry.
// Using a fresh registry per test prevents cross-test pollution of counter values.
func newTestEvents(t *testing.T) (*metrics.Events, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	e := metrics.NewEvents(reg)
	return e, reg
}

// gatherCounter gathers all metrics from reg and returns the value of the counter
// matching the given metric family name and exact set of label key=value pairs.
// Returns -1.0 when the matching series is not found.
func gatherCounter(t *testing.T, reg *prometheus.Registry, name string, want map[string]string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelsMatch(m.GetLabel(), want) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return -1.0
}

// labelsMatch returns true when every key=value in want appears in got.
func labelsMatch(got []*ioprometheusclient.LabelPair, want map[string]string) bool {
	matched := 0
	for _, lp := range got {
		if v, ok := want[lp.GetName()]; ok && v == lp.GetValue() {
			matched++
		}
	}
	return matched == len(want)
}

// --- Authentication event counter tests ---

// TestAuthEvents_LoginSuccess verifies that RecordAuthEvent(login_success)
// increments dzeroth_auth_events_total{event="login_success"} by one.
func TestAuthEvents_LoginSuccess(t *testing.T) {
	e, reg := newTestEvents(t)

	e.RecordAuthEvent(metrics.AuthEventLoginSuccess)

	v := gatherCounter(t, reg, "dzeroth_auth_events_total", map[string]string{"event": "login_success"})
	if v != 1 {
		t.Errorf("dzeroth_auth_events_total{event=login_success}: want 1, got %v", v)
	}
}

// TestAuthEvents_LoginFailure verifies that RecordAuthEvent(login_failure)
// increments dzeroth_auth_events_total{event="login_failure"} by one.
func TestAuthEvents_LoginFailure(t *testing.T) {
	e, reg := newTestEvents(t)

	e.RecordAuthEvent(metrics.AuthEventLoginFailure)

	v := gatherCounter(t, reg, "dzeroth_auth_events_total", map[string]string{"event": "login_failure"})
	if v != 1 {
		t.Errorf("dzeroth_auth_events_total{event=login_failure}: want 1, got %v", v)
	}
}

// TestAuthEvents_TokenRefreshSuccess verifies that RecordAuthEvent(token_refresh_success)
// increments dzeroth_auth_events_total{event="token_refresh_success"} by one.
func TestAuthEvents_TokenRefreshSuccess(t *testing.T) {
	e, reg := newTestEvents(t)

	e.RecordAuthEvent(metrics.AuthEventTokenRefreshSuccess)

	v := gatherCounter(t, reg, "dzeroth_auth_events_total", map[string]string{"event": "token_refresh_success"})
	if v != 1 {
		t.Errorf("dzeroth_auth_events_total{event=token_refresh_success}: want 1, got %v", v)
	}
}

// TestAuthEvents_TokenRefreshFailure verifies that RecordAuthEvent(token_refresh_failure)
// increments dzeroth_auth_events_total{event="token_refresh_failure"} by one.
func TestAuthEvents_TokenRefreshFailure(t *testing.T) {
	e, reg := newTestEvents(t)

	e.RecordAuthEvent(metrics.AuthEventTokenRefreshFailure)

	v := gatherCounter(t, reg, "dzeroth_auth_events_total", map[string]string{"event": "token_refresh_failure"})
	if v != 1 {
		t.Errorf("dzeroth_auth_events_total{event=token_refresh_failure}: want 1, got %v", v)
	}
}

// TestAuthEvents_Logout verifies that RecordAuthEvent(logout)
// increments dzeroth_auth_events_total{event="logout"} by one.
func TestAuthEvents_Logout(t *testing.T) {
	e, reg := newTestEvents(t)

	e.RecordAuthEvent(metrics.AuthEventLogout)

	v := gatherCounter(t, reg, "dzeroth_auth_events_total", map[string]string{"event": "logout"})
	if v != 1 {
		t.Errorf("dzeroth_auth_events_total{event=logout}: want 1, got %v", v)
	}
}

// TestAuthEvents_MultipleEvents verifies that repeated calls accumulate correctly
// and do not contaminate other event label values.
func TestAuthEvents_MultipleEvents(t *testing.T) {
	e, reg := newTestEvents(t)

	e.RecordAuthEvent(metrics.AuthEventLoginSuccess)
	e.RecordAuthEvent(metrics.AuthEventLoginSuccess)
	e.RecordAuthEvent(metrics.AuthEventLoginFailure)

	success := gatherCounter(t, reg, "dzeroth_auth_events_total", map[string]string{"event": "login_success"})
	if success != 2 {
		t.Errorf("login_success: want 2, got %v", success)
	}

	failure := gatherCounter(t, reg, "dzeroth_auth_events_total", map[string]string{"event": "login_failure"})
	if failure != 1 {
		t.Errorf("login_failure: want 1, got %v", failure)
	}
}

// --- Rate-limit event counter tests ---

// TestRateLimitEvents_Rejected verifies that RecordRateLimit with result=rejected
// increments dzeroth_rate_limit_events_total{category=auth_login,result=rejected}.
func TestRateLimitEvents_Rejected(t *testing.T) {
	e, reg := newTestEvents(t)

	e.RecordRateLimit(metrics.RateLimitCategoryAuthLogin, metrics.RateLimitResultRejected)

	v := gatherCounter(t, reg, "dzeroth_rate_limit_events_total", map[string]string{
		"category": "auth_login",
		"result":   "rejected",
	})
	if v != 1 {
		t.Errorf("dzeroth_rate_limit_events_total{category=auth_login,result=rejected}: want 1, got %v", v)
	}
}

// TestRateLimitEvents_Allowed verifies that RecordRateLimit with result=allowed
// increments dzeroth_rate_limit_events_total{category=auth_login,result=allowed}.
func TestRateLimitEvents_Allowed(t *testing.T) {
	e, reg := newTestEvents(t)

	e.RecordRateLimit(metrics.RateLimitCategoryAuthLogin, metrics.RateLimitResultAllowed)

	v := gatherCounter(t, reg, "dzeroth_rate_limit_events_total", map[string]string{
		"category": "auth_login",
		"result":   "allowed",
	})
	if v != 1 {
		t.Errorf("dzeroth_rate_limit_events_total{category=auth_login,result=allowed}: want 1, got %v", v)
	}
}

// TestRateLimitEvents_PostWrite_Rejected verifies counter for post_write category.
func TestRateLimitEvents_PostWrite_Rejected(t *testing.T) {
	e, reg := newTestEvents(t)

	e.RecordRateLimit(metrics.RateLimitCategoryPostWrite, metrics.RateLimitResultRejected)

	v := gatherCounter(t, reg, "dzeroth_rate_limit_events_total", map[string]string{
		"category": "post_write",
		"result":   "rejected",
	})
	if v != 1 {
		t.Errorf("dzeroth_rate_limit_events_total{category=post_write,result=rejected}: want 1, got %v", v)
	}
}

// TestRateLimitEvents_AllowedDoesNotIncrementRejected verifies that allowed and
// rejected counters for the same category are independent.
func TestRateLimitEvents_AllowedDoesNotIncrementRejected(t *testing.T) {
	e, reg := newTestEvents(t)

	e.RecordRateLimit(metrics.RateLimitCategoryAuthLogin, metrics.RateLimitResultAllowed)

	rejected := gatherCounter(t, reg, "dzeroth_rate_limit_events_total", map[string]string{
		"category": "auth_login",
		"result":   "rejected",
	})
	// The rejected series should not have been created yet (returns -1) or have value 0.
	// In either case it must not equal 1.
	if rejected == 1 {
		t.Error("rejected counter must not be incremented when result=allowed")
	}
}

// --- Feed termination event counter tests ---

// TestFeedTermination_Home verifies that RecordFeedTermination(home)
// increments dzeroth_feed_terminations_total{feed_type="home"} by one.
func TestFeedTermination_Home(t *testing.T) {
	e, reg := newTestEvents(t)

	e.RecordFeedTermination(metrics.FeedTypeHome)

	v := gatherCounter(t, reg, "dzeroth_feed_terminations_total", map[string]string{"feed_type": "home"})
	if v != 1 {
		t.Errorf("dzeroth_feed_terminations_total{feed_type=home}: want 1, got %v", v)
	}
}

// TestFeedTermination_HomeAccumulates verifies that multiple terminations
// accumulate (each page response with terminated=true increments the counter).
func TestFeedTermination_HomeAccumulates(t *testing.T) {
	e, reg := newTestEvents(t)

	e.RecordFeedTermination(metrics.FeedTypeHome)
	e.RecordFeedTermination(metrics.FeedTypeHome)
	e.RecordFeedTermination(metrics.FeedTypeHome)

	v := gatherCounter(t, reg, "dzeroth_feed_terminations_total", map[string]string{"feed_type": "home"})
	if v != 3 {
		t.Errorf("dzeroth_feed_terminations_total{feed_type=home}: want 3, got %v", v)
	}
}

// --- Nil-safety tests ---

// TestEvents_NilSafe_RecordAuthEvent verifies that calling RecordAuthEvent on a nil
// *Events is a no-op (does not panic).
func TestEvents_NilSafe_RecordAuthEvent(t *testing.T) {
	var e *metrics.Events
	// Must not panic.
	e.RecordAuthEvent(metrics.AuthEventLoginSuccess)
}

// TestEvents_NilSafe_RecordRateLimit verifies that calling RecordRateLimit on a nil
// *Events is a no-op (does not panic).
func TestEvents_NilSafe_RecordRateLimit(t *testing.T) {
	var e *metrics.Events
	// Must not panic.
	e.RecordRateLimit(metrics.RateLimitCategoryAuthLogin, metrics.RateLimitResultAllowed)
}

// TestEvents_NilSafe_RecordFeedTermination verifies that calling RecordFeedTermination
// on a nil *Events is a no-op (does not panic).
func TestEvents_NilSafe_RecordFeedTermination(t *testing.T) {
	var e *metrics.Events
	// Must not panic.
	e.RecordFeedTermination(metrics.FeedTypeHome)
}
