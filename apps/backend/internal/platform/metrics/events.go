// Package metrics — event counters for authentication, rate-limit, and feed
// termination events. These supplement the existing HTTP metrics (metrics.go)
// with domain-specific observability signals.
//
// Usage:
//
//	// Production (wired in main.go):
//	ev := metrics.NewEvents(prometheus.DefaultRegisterer)
//
//	// Tests (isolated registry, no cross-test pollution):
//	reg := prometheus.NewRegistry()
//	ev := metrics.NewEvents(reg)
package metrics

import "github.com/prometheus/client_golang/prometheus"

// Events holds the Prometheus counters for domain events.
// Create one instance via NewEvents and pass it to services and handlers
// that need to record these events.
//
// All methods are nil-safe: if the receiver is nil, the call is a no-op.
// This allows middleware and services to accept *Events without requiring
// a non-nil value when counters are not needed (e.g., unit tests that do
// not exercise metrics).
type Events struct {
	authEvents      *prometheus.CounterVec
	rateLimitEvents *prometheus.CounterVec
	feedTerminations *prometheus.CounterVec
}

// NewEvents registers the event counters with the provided Prometheus registerer
// and returns an Events instance.
//
// Counter names and labels:
//
//	dzeroth_auth_events_total{event}
//	  event: login_success | login_failure | token_refresh_success |
//	         token_refresh_failure | logout
//
//	dzeroth_rate_limit_events_total{category,result}
//	  result:   allowed | rejected
//	  category: auth_login | auth_register | post_write | follow | block |
//	            bookmark | search | reaction | report | studio
//
//	dzeroth_feed_terminations_total{feed_type}
//	  feed_type: home
//
// Pass prometheus.DefaultRegisterer for production. Pass a fresh
// prometheus.NewRegistry() in tests to prevent cross-test pollution.
func NewEvents(reg prometheus.Registerer) *Events {
	authEvents := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dzeroth_auth_events_total",
			Help: "Total authentication events partitioned by event type.",
		},
		[]string{"event"},
	)

	rateLimitEvents := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dzeroth_rate_limit_events_total",
			Help: "Total rate-limit decisions partitioned by category and result.",
		},
		[]string{"category", "result"},
	)

	feedTerminations := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dzeroth_feed_terminations_total",
			Help: "Total feed termination events partitioned by feed type.",
		},
		[]string{"feed_type"},
	)

	reg.MustRegister(authEvents, rateLimitEvents, feedTerminations)

	return &Events{
		authEvents:       authEvents,
		rateLimitEvents:  rateLimitEvents,
		feedTerminations: feedTerminations,
	}
}

// --- Authentication event constants ---

const (
	AuthEventLoginSuccess          = "login_success"
	AuthEventLoginFailure          = "login_failure"
	AuthEventTokenRefreshSuccess   = "token_refresh_success"
	AuthEventTokenRefreshFailure   = "token_refresh_failure"
	AuthEventLogout                = "logout"
)

// --- Rate-limit category constants ---

const (
	RateLimitCategoryAuthLogin    = "auth_login"
	RateLimitCategoryAuthRegister = "auth_register"
	RateLimitCategoryPostWrite    = "post_write"
	RateLimitCategoryFollow       = "follow"
	RateLimitCategoryBlock        = "block"
	RateLimitCategoryBookmark     = "bookmark"
	RateLimitCategorySearch       = "search"
	RateLimitCategoryReaction     = "reaction"
	RateLimitCategoryReport       = "report"
	RateLimitCategoryStudio       = "studio"
)

// --- Rate-limit result constants ---

const (
	RateLimitResultAllowed  = "allowed"
	RateLimitResultRejected = "rejected"
)

// --- Feed type constants ---

const (
	FeedTypeHome = "home"
)

// RecordAuthEvent increments the auth event counter for the given event label.
// Safe to call on a nil *Events.
func (e *Events) RecordAuthEvent(event string) {
	if e == nil {
		return
	}
	e.authEvents.WithLabelValues(event).Inc()
}

// RecordRateLimit increments the rate-limit counter for the given category and result.
// result must be RateLimitResultAllowed or RateLimitResultRejected.
// Safe to call on a nil *Events.
func (e *Events) RecordRateLimit(category, result string) {
	if e == nil {
		return
	}
	e.rateLimitEvents.WithLabelValues(category, result).Inc()
}

// RecordFeedTermination increments the feed termination counter for the given feed type.
// Safe to call on a nil *Events.
func (e *Events) RecordFeedTermination(feedType string) {
	if e == nil {
		return
	}
	e.feedTerminations.WithLabelValues(feedType).Inc()
}
