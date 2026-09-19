package metrics_test

import (
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/buildinfo"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/metrics"
)

// TestRegisterBuildInfo_GaugeValue confirms that after registration the single
// gauge series has a value of exactly 1.0.
func TestRegisterBuildInfo_GaugeValue(t *testing.T) {
	reg := prometheus.NewRegistry()
	g := metrics.RegisterBuildInfo(reg, "v1.0.0", "abc1234", "2026-09-19T12:00:00Z")

	var m dto.Metric
	if err := g.Write(&m); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := m.GetGauge().GetValue(); got != 1.0 {
		t.Errorf("gauge value: want 1.0, got %f", got)
	}
}

// TestRegisterBuildInfo_Labels confirms that the metric family carries labels
// "version", "commit", and "build_time" with the exact values supplied.
func TestRegisterBuildInfo_Labels(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics.RegisterBuildInfo(reg, "v2.3.4", "def5678", "2026-01-01T00:00:00Z")

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	var mf *dto.MetricFamily
	for _, f := range mfs {
		if f.GetName() == "dzeroth_build_info" {
			mf = f
			break
		}
	}
	if mf == nil {
		t.Fatal("dzeroth_build_info metric family not found")
	}

	if len(mf.GetMetric()) != 1 {
		t.Fatalf("expected 1 metric series, got %d", len(mf.GetMetric()))
	}

	labelMap := make(map[string]string)
	for _, lp := range mf.GetMetric()[0].GetLabel() {
		labelMap[lp.GetName()] = lp.GetValue()
	}

	if got := labelMap["version"]; got != "v2.3.4" {
		t.Errorf("label version: want %q, got %q", "v2.3.4", got)
	}
	if got := labelMap["commit"]; got != "def5678" {
		t.Errorf("label commit: want %q, got %q", "def5678", got)
	}
	if got := labelMap["build_time"]; got != "2026-01-01T00:00:00Z" {
		t.Errorf("label build_time: want %q, got %q", "2026-01-01T00:00:00Z", got)
	}
}

// TestRegisterBuildInfo_DefaultsAreNonEmpty confirms that using the package
// default values from buildinfo produces non-empty label values.
func TestRegisterBuildInfo_DefaultsAreNonEmpty(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics.RegisterBuildInfo(reg, buildinfo.Version, buildinfo.Commit, buildinfo.BuildTime)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	var mf *dto.MetricFamily
	for _, f := range mfs {
		if f.GetName() == "dzeroth_build_info" {
			mf = f
			break
		}
	}
	if mf == nil {
		t.Fatal("dzeroth_build_info metric family not found")
	}

	labelMap := make(map[string]string)
	for _, lp := range mf.GetMetric()[0].GetLabel() {
		labelMap[lp.GetName()] = lp.GetValue()
	}

	for _, key := range []string{"version", "commit", "build_time"} {
		if labelMap[key] == "" {
			t.Errorf("label %q is empty when using buildinfo defaults", key)
		}
	}
}

// TestRegisterBuildInfo_ExactlyOneSeries confirms that after one registration
// the gatherer returns exactly one MetricFamily named "dzeroth_build_info"
// with exactly one Metric entry.
func TestRegisterBuildInfo_ExactlyOneSeries(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics.RegisterBuildInfo(reg, "v1.0.0", "abc1234", "2026-09-19T12:00:00Z")

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	var found int
	for _, f := range mfs {
		if f.GetName() == "dzeroth_build_info" {
			found++
			if n := len(f.GetMetric()); n != 1 {
				t.Errorf("expected exactly 1 Metric in MetricFamily, got %d", n)
			}
		}
	}
	if found != 1 {
		t.Errorf("expected exactly 1 MetricFamily named dzeroth_build_info, found %d", found)
	}
}

// TestRegisterBuildInfo_NoRequestCardinality confirms that calling RegisterBuildInfo
// a second time on the same registry returns an error (panics via MustRegister),
// proving the metric does not accumulate per-request series.
func TestRegisterBuildInfo_NoRequestCardinality(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics.RegisterBuildInfo(reg, "v1.0.0", "abc1234", "2026-09-19T12:00:00Z")

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from duplicate registration, got none")
		}
	}()

	// This second call must panic via MustRegister because the GaugeVec name
	// is already registered on this registry.
	metrics.RegisterBuildInfo(reg, "v2.0.0", "xyz9999", "2026-09-20T00:00:00Z")
}

// TestRegisterBuildInfo_NoPanic_EmptyStrings confirms that calling RegisterBuildInfo
// with empty strings for all three arguments does not panic. Safe defaults are
// substituted instead of propagating empty label values.
func TestRegisterBuildInfo_NoPanic_EmptyStrings(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic with empty strings: %v", r)
		}
	}()

	reg := prometheus.NewRegistry()
	g := metrics.RegisterBuildInfo(reg, "", "", "")

	// The returned gauge must still be valid and set to 1.0.
	var m dto.Metric
	if err := g.Write(&m); err != nil {
		t.Fatalf("Write after empty-string registration: %v", err)
	}
	if got := m.GetGauge().GetValue(); got != 1.0 {
		t.Errorf("gauge value after empty-string registration: want 1.0, got %f", got)
	}
}
