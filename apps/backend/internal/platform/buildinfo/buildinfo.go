// Package buildinfo holds build-time identity variables populated via -ldflags.
// When no ldflags are supplied (local/development builds), the defaults are used
// and the application compiles and runs without modification.
//
// Production CI/CD injection example:
//
//	go build -ldflags "\
//	  -X github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/buildinfo.Version=1.0.0 \
//	  -X github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/buildinfo.Commit=abc1234 \
//	  -X github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/buildinfo.BuildTime=2026-09-19T12:00:00Z" \
//	  ./cmd/api
package buildinfo

// Variables populated at build time via -ldflags.
// Defaults are used for local/development builds.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)
