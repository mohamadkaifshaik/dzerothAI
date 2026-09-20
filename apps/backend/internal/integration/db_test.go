//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// ---------------------------------------------------------------------------
// PostgreSQL connectivity
// ---------------------------------------------------------------------------

// TestDB_Connect verifies that the integration test database is reachable and
// that a Ping succeeds after pool creation.
func TestDB_Connect(t *testing.T) {
	pool := connectTestDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pool.Ping failed after Connect: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Migrations
// ---------------------------------------------------------------------------

// TestDB_MigrationsIdempotent verifies that running migrations twice does not
// return an error. golang-migrate returns ErrNoChange on the second run, which
// runTestMigrations must ignore.
func TestDB_MigrationsIdempotent(t *testing.T) {
	// connectTestDB already runs migrations once.
	_ = connectTestDB(t)

	// Running migrations a second time against the same database must succeed.
	if err := runTestMigrations(testPostgresDSN()); err != nil {
		t.Fatalf("second migration run returned an error: %v", err)
	}
}

// TestDB_MigrationsCreatedExpectedTables verifies that the schema produced by
// the migrations includes the core tables required by Dzeroth. This guards
// against a migration that silently skips or drops tables.
func TestDB_MigrationsCreatedExpectedTables(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()

	tables := []string{
		"users", "sessions", "posts", "follows",
		"bookmarks", "notifications", "reports",
	}

	for _, table := range tables {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("checking table %q: %v", table, err)
		}
		if !exists {
			t.Errorf("expected table %q to exist after migrations", table)
		}
	}
}

// ---------------------------------------------------------------------------
// User CRUD
// ---------------------------------------------------------------------------

// TestDB_CreateUser verifies that a user can be inserted and retrieved by email.
func TestDB_CreateUser(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()

	id := uuid.New()
	email := "testuser_" + id.String()[:8] + "@example.com"
	input := &auth.UserCreateInput{
		ID:           id,
		Handle:       "testuser_" + id.String()[:8],
		DisplayName:  "Test User",
		Email:        email,
		PasswordHash: "$2a$12$placeholderhashforintegrationtestonly0000000000000000",
	}

	got, err := auth.CreateUser(ctx, pool, input)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if got.ID != id {
		t.Errorf("CreateUser returned ID %v, want %v", got.ID, id)
	}
	if got.Handle != input.Handle {
		t.Errorf("CreateUser returned Handle %q, want %q", got.Handle, input.Handle)
	}
	if got.Email != input.Email {
		t.Errorf("CreateUser returned Email %q, want %q", got.Email, input.Email)
	}

	// Verify retrieval by email.
	fetched, err := auth.GetUserByEmail(ctx, pool, email)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if fetched.ID != id {
		t.Errorf("GetUserByEmail ID = %v, want %v", fetched.ID, id)
	}
}

// TestDB_CreateUser_DuplicateEmail verifies that inserting two users with the
// same email returns ErrDuplicateEmail.
func TestDB_CreateUser_DuplicateEmail(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()

	email := "dup_email_" + uuid.New().String()[:8] + "@example.com"
	makeInput := func() *auth.UserCreateInput {
		id := uuid.New()
		return &auth.UserCreateInput{
			ID:           id,
			Handle:       "duphandle_" + id.String()[:8],
			DisplayName:  "Dup",
			Email:        email,
			PasswordHash: "$2a$12$placeholderhashforintegrationtestonly0000000000000000",
		}
	}

	if _, err := auth.CreateUser(ctx, pool, makeInput()); err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}

	_, err := auth.CreateUser(ctx, pool, makeInput())
	if err == nil {
		t.Fatal("expected ErrDuplicateEmail, got nil")
	}
	if err != auth.ErrDuplicateEmail {
		t.Errorf("got error %v, want ErrDuplicateEmail", err)
	}
}

// TestDB_CreateUser_DuplicateHandle verifies that inserting two users with the
// same handle returns ErrDuplicateHandle.
func TestDB_CreateUser_DuplicateHandle(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()

	handle := "duphandle_" + uuid.New().String()[:8]
	makeInput := func() *auth.UserCreateInput {
		id := uuid.New()
		return &auth.UserCreateInput{
			ID:           id,
			Handle:       handle,
			DisplayName:  "Dup",
			Email:        "uniq_" + id.String()[:8] + "@example.com",
			PasswordHash: "$2a$12$placeholderhashforintegrationtestonly0000000000000000",
		}
	}

	if _, err := auth.CreateUser(ctx, pool, makeInput()); err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}

	_, err := auth.CreateUser(ctx, pool, makeInput())
	if err == nil {
		t.Fatal("expected ErrDuplicateHandle, got nil")
	}
	if err != auth.ErrDuplicateHandle {
		t.Errorf("got error %v, want ErrDuplicateHandle", err)
	}
}

// ---------------------------------------------------------------------------
// Session CRUD
// ---------------------------------------------------------------------------

// TestDB_CreateAndGetSession verifies that a session can be inserted and then
// retrieved by its token hash.
func TestDB_CreateAndGetSession(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()

	// Create a user to attach the session to.
	userID := uuid.New()
	_, err := auth.CreateUser(ctx, pool, &auth.UserCreateInput{
		ID:           userID,
		Handle:       "sessuser_" + userID.String()[:8],
		DisplayName:  "Session User",
		Email:        "sessuser_" + userID.String()[:8] + "@example.com",
		PasswordHash: "$2a$12$placeholderhashforintegrationtestonly0000000000000000",
	})
	if err != nil {
		t.Fatalf("CreateUser for session test: %v", err)
	}

	sessionID := uuid.New()
	tokenHash := "testhash_" + sessionID.String()
	ip := "127.0.0.1"
	ua := "integration-test-agent"

	err = auth.CreateSession(ctx, pool, &auth.SessionCreateInput{
		ID:        sessionID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour),
		IPAddress: &ip,
		UserAgent: &ua,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Retrieve by token hash.
	retrieved, err := auth.GetSessionByHash(ctx, pool, tokenHash)
	if err != nil {
		t.Fatalf("GetSessionByHash: %v", err)
	}
	if retrieved.ID != sessionID {
		t.Errorf("GetSessionByHash ID = %v, want %v", retrieved.ID, sessionID)
	}
	if retrieved.UserID != userID {
		t.Errorf("GetSessionByHash UserID = %v, want %v", retrieved.UserID, userID)
	}
}

// TestDB_GetSessionByHash_NotFound verifies that querying a non-existent
// token hash returns ErrNotFound.
func TestDB_GetSessionByHash_NotFound(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()

	_, err := auth.GetSessionByHash(ctx, pool, "nonexistent_token_hash_"+uuid.New().String())
	if err == nil {
		t.Fatal("expected ErrNotFound, got nil")
	}
	if err != auth.ErrNotFound {
		t.Errorf("got error %v, want ErrNotFound", err)
	}
}

// TestDB_DeleteSession verifies that a session can be deleted and is no longer
// retrievable afterwards.
func TestDB_DeleteSession(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()

	userID := uuid.New()
	_, err := auth.CreateUser(ctx, pool, &auth.UserCreateInput{
		ID:           userID,
		Handle:       "deluser_" + userID.String()[:8],
		DisplayName:  "Del User",
		Email:        "deluser_" + userID.String()[:8] + "@example.com",
		PasswordHash: "$2a$12$placeholderhashforintegrationtestonly0000000000000000",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	sessionID := uuid.New()
	tokenHash := "delhash_" + sessionID.String()
	if err := auth.CreateSession(ctx, pool, &auth.SessionCreateInput{
		ID:        sessionID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := auth.DeleteSession(ctx, pool, sessionID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	_, err = auth.GetSessionByHash(ctx, pool, tokenHash)
	if err == nil {
		t.Fatal("expected ErrNotFound after DeleteSession, got nil")
	}
	if err != auth.ErrNotFound {
		t.Errorf("got error %v, want ErrNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// Constraint: handle is case-insensitively unique
// ---------------------------------------------------------------------------

// TestDB_HandleIsCaseInsensitivelyUnique verifies the lower(handle) unique
// index: inserting "ALICE" and "alice" must fail with ErrDuplicateHandle.
func TestDB_HandleIsCaseInsensitivelyUnique(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()

	base := "caseuser_" + uuid.New().String()[:8]

	_, err := auth.CreateUser(ctx, pool, &auth.UserCreateInput{
		ID:           uuid.New(),
		Handle:       strings.ToUpper(base),
		DisplayName:  "Case Upper",
		Email:        "case_upper_" + uuid.New().String()[:8] + "@example.com",
		PasswordHash: "$2a$12$placeholderhashforintegrationtestonly0000000000000000",
	})
	if err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}

	_, err = auth.CreateUser(ctx, pool, &auth.UserCreateInput{
		ID:           uuid.New(),
		Handle:       strings.ToLower(base),
		DisplayName:  "Case Lower",
		Email:        "case_lower_" + uuid.New().String()[:8] + "@example.com",
		PasswordHash: "$2a$12$placeholderhashforintegrationtestonly0000000000000000",
	})
	if err == nil {
		t.Fatal("expected ErrDuplicateHandle for case-insensitive duplicate, got nil")
	}
	if err != auth.ErrDuplicateHandle {
		t.Errorf("got error %v, want ErrDuplicateHandle", err)
	}
}
