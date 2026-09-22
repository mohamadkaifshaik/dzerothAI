package feed

// Benchmarks for feed-layer operations that are deterministic and require no
// live database or Redis connection.
//
// Run with:
//
//	go test -bench=. -benchmem ./internal/feed/...
//
// These benchmarks establish a CPU and allocation baseline for the cursor
// encode/decode cycle, which is exercised on every paginated feed request.
// They serve as a canary for regressions introduced by changes to the
// FeedCursor encoding path.

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// BenchmarkFeedCursorEncode measures the cost of encoding a FeedCursor to
// its opaque base64url string representation.  This path is hit at the end
// of every paginated feed query when a next_cursor is produced.
func BenchmarkFeedCursorEncode(b *testing.B) {
	id, err := uuid.NewV7()
	if err != nil {
		b.Fatalf("uuid.NewV7: %v", err)
	}
	cursor := post.FeedCursor{
		AfterID:   id,
		Timestamp: time.Now().UTC(),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = cursor.Encode()
	}
}

// BenchmarkFeedCursorDecode measures the cost of decoding an opaque
// base64url cursor string back into a FeedCursor.  This path is hit on
// every paginated feed request that carries a cursor query parameter.
func BenchmarkFeedCursorDecode(b *testing.B) {
	id, err := uuid.NewV7()
	if err != nil {
		b.Fatalf("uuid.NewV7: %v", err)
	}
	cursor := post.FeedCursor{
		AfterID:   id,
		Timestamp: time.Now().UTC(),
	}
	encoded := cursor.Encode()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = post.DecodeCursor(encoded)
	}
}

// BenchmarkFeedCursorRoundTrip measures the combined encode+decode cycle.
// This is the realistic worst-case: every request that reads and writes a
// cursor pays both costs.
func BenchmarkFeedCursorRoundTrip(b *testing.B) {
	id, err := uuid.NewV7()
	if err != nil {
		b.Fatalf("uuid.NewV7: %v", err)
	}
	cursor := post.FeedCursor{
		AfterID:   id,
		Timestamp: time.Now().UTC(),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		encoded := cursor.Encode()
		_, _ = post.DecodeCursor(encoded)
	}
}
