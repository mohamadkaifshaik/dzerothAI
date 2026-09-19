// Package metrics — build identity gauge.
//
// RegisterBuildInfo registers the dzeroth_build_info metric and sets it to 1.0
// with the supplied build metadata labels. This follows the standard Prometheus
// build-info pattern: a single gauge series permanently set to 1, with labels
// carrying the version, commit, and build timestamp of the running binary.
//
// The metric produces exactly one series per process lifetime. It must not be
// called with per-request or dynamic values — cardinality is intentionally zero
// beyond the three static build-identity labels.
//
// Usage:
//
//	// Production (wired in main.go alongside NewEvents / NewInfraMetrics):
//	metrics.RegisterBuildInfo(prometheus.DefaultRegisterer,
//	    buildinfo.Version, buildinfo.Commit, buildinfo.BuildTime)
//
//	// Tests (isolated registry, no cross-test pollution):
//	reg := prometheus.NewRegistry()
//	g := metrics.RegisterBuildInfo(reg, "v1.2.3", "abc1234", "2026-09-19T12:00:00Z")
package metrics

import "github.com/prometheus/client_golang/prometheus"

const (
	buildInfoDefaultVersion   = "dev"
	buildInfoDefaultCommit    = "unknown"
	buildInfoDefaultBuildTime = "unknown"
)

// RegisterBuildInfo registers the dzeroth_build_info GaugeVec with the provided
// Prometheus registerer, sets the single gauge series (identified by the three
// build-identity labels) to 1.0, and returns the gauge for test assertions.
//
// Safe defaults are substituted when any of the three string arguments is empty:
//   - version  → "dev"
//   - commit   → "unknown"
//   - buildTime → "unknown"
//
// This function must be called exactly once per prometheus.Registerer. Calling it
// a second time with the same registerer will cause MustRegister to panic (this is
// intentional: it proves there is no per-request cardinality).
func RegisterBuildInfo(reg prometheus.Registerer, version, commit, buildTime string) prometheus.Gauge {
	if version == "" {
		version = buildInfoDefaultVersion
	}
	if commit == "" {
		commit = buildInfoDefaultCommit
	}
	if buildTime == "" {
		buildTime = buildInfoDefaultBuildTime
	}

	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dzeroth_build_info",
			Help: "Build information for the running Dzeroth instance.",
		},
		[]string{"version", "commit", "build_time"},
	)

	reg.MustRegister(buildInfo)

	g := buildInfo.WithLabelValues(version, commit, buildTime)
	g.Set(1.0)
	return g
}
