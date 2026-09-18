//go:build tools

// Package tools pins build and runtime dependencies so go mod tidy does not
// remove them before the internal packages that will import them are written.
// This file is excluded from normal compilation by the "tools" build tag.
package tools

import (
	_ "github.com/go-chi/chi/v5"
	_ "github.com/golang-jwt/jwt/v5"
	_ "github.com/golang-migrate/migrate/v4"
	_ "github.com/google/uuid"
	_ "github.com/jackc/pgx/v5"
	_ "github.com/redis/go-redis/v9"
	_ "go.uber.org/zap"
	_ "golang.org/x/crypto/bcrypt"
)
