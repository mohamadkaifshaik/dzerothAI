// Package metrics — infrastructure gauges for PostgreSQL connection pool and Redis
// availability. These gauges are updated by piggybacking on the existing /health and
// /readyz handler calls; no separate background goroutine or additional Redis command
// is introduced.
//
// Usage:
//
//	// Production (wired in main.go):
//	infra := metrics.NewInfraMetrics(prometheus.DefaultRegisterer)
//
//	// Tests (isolated registry, no cross-test pollution):
//	reg := prometheus.NewRegistry()
//	infra := metrics.NewInfraMetrics(reg)
package metrics

import "github.com/prometheus/client_golang/prometheus"

// DBPoolStat is satisfied by *pgxpool.Stat (github.com/jackc/pgx/v5/pgxpool).
// It is defined here as a local interface so that callers in cmd/api can pass
// pool.Stat() directly, and tests can inject a fake without importing pgxpool.
type DBPoolStat interface {
	TotalConns() int32
	AcquiredConns() int32
	IdleConns() int32
	MaxConns() int32
}

// InfraMetrics holds the Prometheus gauges for infrastructure health.
// Create one instance via NewInfraMetrics and pass it to the health/readyz handler
// builders so they can call UpdateDBPool and SetRedisUp after their dependency checks.
//
// All methods are nil-safe: if the receiver is nil, the call is a no-op.
// This allows handlers to accept *InfraMetrics without requiring a non-nil value
// in tests that do not exercise metrics.
type InfraMetrics struct {
	dbPoolTotal    prometheus.Gauge
	dbPoolAcquired prometheus.Gauge
	dbPoolIdle     prometheus.Gauge
	dbPoolMax      prometheus.Gauge
	redisUp        prometheus.Gauge
}

// NewInfraMetrics registers the infrastructure gauges with the provided Prometheus
// registerer and returns an InfraMetrics instance.
//
// Gauge names and semantics:
//
//	dzeroth_db_pool_connections_total    — TotalConns: idle + acquired + constructing
//	dzeroth_db_pool_connections_acquired — AcquiredConns: connections currently in use
//	dzeroth_db_pool_connections_idle     — IdleConns: connections available in the pool
//	dzeroth_db_pool_connections_max      — MaxConns: configured pool maximum
//	dzeroth_redis_up                     — 1.0 = available, 0.0 = unavailable
//
// No labels are used (zero cardinality). These gauges describe pool state, not
// per-request data.
//
// Pass prometheus.DefaultRegisterer for production. Pass a fresh
// prometheus.NewRegistry() in tests to prevent cross-test pollution.
func NewInfraMetrics(reg prometheus.Registerer) *InfraMetrics {
	dbPoolTotal := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dzeroth_db_pool_connections_total",
		Help: "Current total connections in the PostgreSQL pool (idle + acquired + constructing).",
	})

	dbPoolAcquired := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dzeroth_db_pool_connections_acquired",
		Help: "Current number of PostgreSQL connections actively in use by the application.",
	})

	dbPoolIdle := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dzeroth_db_pool_connections_idle",
		Help: "Current number of idle PostgreSQL connections available in the pool.",
	})

	dbPoolMax := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dzeroth_db_pool_connections_max",
		Help: "Configured maximum connections for the PostgreSQL pool.",
	})

	redisUp := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dzeroth_redis_up",
		Help: "Redis availability: 1 = available (last health check succeeded), 0 = unavailable.",
	})

	reg.MustRegister(dbPoolTotal, dbPoolAcquired, dbPoolIdle, dbPoolMax, redisUp)

	return &InfraMetrics{
		dbPoolTotal:    dbPoolTotal,
		dbPoolAcquired: dbPoolAcquired,
		dbPoolIdle:     dbPoolIdle,
		dbPoolMax:      dbPoolMax,
		redisUp:        redisUp,
	}
}

// UpdateDBPool records the current pool statistics from the provided DBPoolStat.
// Call this after a successful DB ping in the /health or /readyz handler.
// The stat read is non-blocking (pgxpool.Pool.Stat() reads in-memory pool state).
// Safe to call on a nil *InfraMetrics.
func (im *InfraMetrics) UpdateDBPool(stat DBPoolStat) {
	if im == nil {
		return
	}
	im.dbPoolTotal.Set(float64(stat.TotalConns()))
	im.dbPoolAcquired.Set(float64(stat.AcquiredConns()))
	im.dbPoolIdle.Set(float64(stat.IdleConns()))
	im.dbPoolMax.Set(float64(stat.MaxConns()))
}

// SetRedisUp records the current Redis availability state.
// available=true sets dzeroth_redis_up to 1.0; available=false sets it to 0.0.
// Call this after the Redis availability check in /health or /readyz handlers.
// Safe to call on a nil *InfraMetrics.
func (im *InfraMetrics) SetRedisUp(available bool) {
	if im == nil {
		return
	}
	if available {
		im.redisUp.Set(1.0)
	} else {
		im.redisUp.Set(0.0)
	}
}
