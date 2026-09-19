package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/metrics"
)

// fakePoolStat implements metrics.DBPoolStat for use in tests.
// It holds explicit int32 values for all four pool stat fields so that
// tests can inject any combination without requiring a real *pgxpool.Pool.
type fakePoolStat struct {
	total    int32
	acquired int32
	idle     int32
	max      int32
}

func (f *fakePoolStat) TotalConns() int32    { return f.total }
func (f *fakePoolStat) AcquiredConns() int32 { return f.acquired }
func (f *fakePoolStat) IdleConns() int32     { return f.idle }
func (f *fakePoolStat) MaxConns() int32      { return f.max }

// newTestInfra creates an InfraMetrics instance backed by a fresh isolated registry.
// Using a fresh registry per test prevents cross-test pollution of gauge values.
func newTestInfra(t *testing.T) (*metrics.InfraMetrics, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	im := metrics.NewInfraMetrics(reg)
	return im, reg
}

// gatherGauge gathers all metrics from reg and returns the current value of the gauge
// with the given metric family name. Returns -999.0 if the gauge is not found so that
// tests can distinguish "not found" from a legitimate 0.0 or negative value.
func gatherGauge(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
		metrics := mf.GetMetric()
		if len(metrics) == 0 {
			t.Fatalf("gauge %s: found family but no metric series", name)
		}
		return metrics[0].GetGauge().GetValue()
	}
	return -999.0
}

// --- DB pool gauge tests ---

// TestInfraMetrics_DBPool_AllFour verifies that UpdateDBPool sets all four gauges to
// the values returned by the provided DBPoolStat in a single call.
func TestInfraMetrics_DBPool_AllFour(t *testing.T) {
	im, reg := newTestInfra(t)

	stat := &fakePoolStat{total: 10, acquired: 4, idle: 6, max: 25}
	im.UpdateDBPool(stat)

	cases := []struct {
		name string
		want float64
	}{
		{"dzeroth_db_pool_connections_total", 10},
		{"dzeroth_db_pool_connections_acquired", 4},
		{"dzeroth_db_pool_connections_idle", 6},
		{"dzeroth_db_pool_connections_max", 25},
	}

	for _, tc := range cases {
		got := gatherGauge(t, reg, tc.name)
		if got != tc.want {
			t.Errorf("%s: want %.0f, got %.0f", tc.name, tc.want, got)
		}
	}
}

// TestInfraMetrics_DBPool_TotalConns verifies that TotalConns maps to
// dzeroth_db_pool_connections_total.
func TestInfraMetrics_DBPool_TotalConns(t *testing.T) {
	im, reg := newTestInfra(t)

	im.UpdateDBPool(&fakePoolStat{total: 7})

	got := gatherGauge(t, reg, "dzeroth_db_pool_connections_total")
	if got != 7 {
		t.Errorf("dzeroth_db_pool_connections_total: want 7, got %.0f", got)
	}
}

// TestInfraMetrics_DBPool_AcquiredConns verifies that AcquiredConns maps to
// dzeroth_db_pool_connections_acquired.
func TestInfraMetrics_DBPool_AcquiredConns(t *testing.T) {
	im, reg := newTestInfra(t)

	im.UpdateDBPool(&fakePoolStat{acquired: 3})

	got := gatherGauge(t, reg, "dzeroth_db_pool_connections_acquired")
	if got != 3 {
		t.Errorf("dzeroth_db_pool_connections_acquired: want 3, got %.0f", got)
	}
}

// TestInfraMetrics_DBPool_IdleConns verifies that IdleConns maps to
// dzeroth_db_pool_connections_idle.
func TestInfraMetrics_DBPool_IdleConns(t *testing.T) {
	im, reg := newTestInfra(t)

	im.UpdateDBPool(&fakePoolStat{idle: 5})

	got := gatherGauge(t, reg, "dzeroth_db_pool_connections_idle")
	if got != 5 {
		t.Errorf("dzeroth_db_pool_connections_idle: want 5, got %.0f", got)
	}
}

// TestInfraMetrics_DBPool_MaxConns verifies that MaxConns maps to
// dzeroth_db_pool_connections_max.
func TestInfraMetrics_DBPool_MaxConns(t *testing.T) {
	im, reg := newTestInfra(t)

	im.UpdateDBPool(&fakePoolStat{max: 25})

	got := gatherGauge(t, reg, "dzeroth_db_pool_connections_max")
	if got != 25 {
		t.Errorf("dzeroth_db_pool_connections_max: want 25, got %.0f", got)
	}
}

// --- Redis gauge tests ---

// TestInfraMetrics_Redis_Up verifies that SetRedisUp(true) sets dzeroth_redis_up to 1.0.
func TestInfraMetrics_Redis_Up(t *testing.T) {
	im, reg := newTestInfra(t)

	im.SetRedisUp(true)

	got := gatherGauge(t, reg, "dzeroth_redis_up")
	if got != 1.0 {
		t.Errorf("dzeroth_redis_up after SetRedisUp(true): want 1.0, got %v", got)
	}
}

// TestInfraMetrics_Redis_Down verifies that SetRedisUp(false) sets dzeroth_redis_up to 0.0.
func TestInfraMetrics_Redis_Down(t *testing.T) {
	im, reg := newTestInfra(t)

	im.SetRedisUp(false)

	got := gatherGauge(t, reg, "dzeroth_redis_up")
	if got != 0.0 {
		t.Errorf("dzeroth_redis_up after SetRedisUp(false): want 0.0, got %v", got)
	}
}

// TestInfraMetrics_Redis_Transition verifies that the Redis gauge correctly tracks
// availability transitions: true → false → true produces the expected final value
// and intermediate states are also correct.
func TestInfraMetrics_Redis_Transition(t *testing.T) {
	im, reg := newTestInfra(t)

	// Start available.
	im.SetRedisUp(true)
	if v := gatherGauge(t, reg, "dzeroth_redis_up"); v != 1.0 {
		t.Errorf("after true: want 1.0, got %v", v)
	}

	// Become unavailable.
	im.SetRedisUp(false)
	if v := gatherGauge(t, reg, "dzeroth_redis_up"); v != 0.0 {
		t.Errorf("after false: want 0.0, got %v", v)
	}

	// Recover.
	im.SetRedisUp(true)
	if v := gatherGauge(t, reg, "dzeroth_redis_up"); v != 1.0 {
		t.Errorf("after recovery: want 1.0, got %v", v)
	}
}

// --- Nil-safety tests ---

// TestInfraMetrics_NilSafe_UpdateDBPool verifies that calling UpdateDBPool on a nil
// *InfraMetrics is a no-op and does not panic.
func TestInfraMetrics_NilSafe_UpdateDBPool(t *testing.T) {
	var im *metrics.InfraMetrics
	// Must not panic.
	im.UpdateDBPool(&fakePoolStat{total: 5, acquired: 2, idle: 3, max: 25})
}

// TestInfraMetrics_NilSafe_SetRedisUp verifies that calling SetRedisUp on a nil
// *InfraMetrics is a no-op and does not panic.
func TestInfraMetrics_NilSafe_SetRedisUp(t *testing.T) {
	var im *metrics.InfraMetrics
	// Must not panic.
	im.SetRedisUp(true)
	im.SetRedisUp(false)
}

// --- Cardinality tests ---

// TestInfraMetrics_NoLabels verifies that all five infrastructure gauges have exactly
// zero label dimensions (no labels). This ensures the gauges have fixed cardinality
// of 1 series each rather than growing with usage patterns.
func TestInfraMetrics_NoLabels(t *testing.T) {
	im, reg := newTestInfra(t)

	// Populate all gauges so their series are created in the registry.
	im.UpdateDBPool(&fakePoolStat{total: 1, acquired: 1, idle: 0, max: 25})
	im.SetRedisUp(true)

	gaugeNames := []string{
		"dzeroth_db_pool_connections_total",
		"dzeroth_db_pool_connections_acquired",
		"dzeroth_db_pool_connections_idle",
		"dzeroth_db_pool_connections_max",
		"dzeroth_redis_up",
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	found := make(map[string]bool, len(gaugeNames))
	for _, mf := range families {
		for _, wantName := range gaugeNames {
			if mf.GetName() != wantName {
				continue
			}
			found[wantName] = true
			for _, m := range mf.GetMetric() {
				if len(m.GetLabel()) != 0 {
					t.Errorf("%s: expected 0 labels, got %d: %v",
						wantName, len(m.GetLabel()), m.GetLabel())
				}
			}
		}
	}

	for _, name := range gaugeNames {
		if !found[name] {
			t.Errorf("gauge %s not found in registry after update", name)
		}
	}
}
