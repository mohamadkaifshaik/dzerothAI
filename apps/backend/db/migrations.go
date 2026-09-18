// Package db exposes the embedded migration SQL files for use by cmd/api.
// The embed directive is valid here because this file lives in the same directory
// as the migrations sub-directory (apps/backend/db/).
package db

import "embed"

// MigrationsFS contains all SQL migration files.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
