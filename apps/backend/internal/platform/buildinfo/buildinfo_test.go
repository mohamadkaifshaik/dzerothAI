package buildinfo_test

import (
	"testing"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/buildinfo"
)

// TestBuildInfo_DefaultsAreStable confirms that the package-level default values
// have not been accidentally changed. These defaults are the stable local-build
// values and are referenced by the metrics tests.
func TestBuildInfo_DefaultsAreStable(t *testing.T) {
	if buildinfo.Version != "dev" {
		t.Errorf("buildinfo.Version: want %q, got %q", "dev", buildinfo.Version)
	}
	if buildinfo.Commit != "unknown" {
		t.Errorf("buildinfo.Commit: want %q, got %q", "unknown", buildinfo.Commit)
	}
	if buildinfo.BuildTime != "unknown" {
		t.Errorf("buildinfo.BuildTime: want %q, got %q", "unknown", buildinfo.BuildTime)
	}
}
