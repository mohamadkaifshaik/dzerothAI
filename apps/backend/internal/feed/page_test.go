package feed

// Unit tests for the home feed pagination helper used by the hard-depth
// ListHomeTimeline query. The SQL window itself is covered by the integration
// tests in internal/integration.

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

func makeFeedPagePosts(t *testing.T, n int) []post.Post {
	t.Helper()
	base := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	posts := make([]post.Post, n)
	for i := range posts {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatalf("uuid.NewV7: %v", err)
		}
		posts[i] = post.Post{ID: id, CreatedAt: base.Add(-time.Duration(i) * time.Second)}
	}
	return posts
}

// TestBuildFeedPage_ExtraRow_NotTerminatedWithCursor verifies that pageSize+1
// rows yield a full page, terminated=false, and a cursor at the last row.
func TestBuildFeedPage_ExtraRow_NotTerminatedWithCursor(t *testing.T) {
	page, cursor, terminated, err := buildFeedPage(makeFeedPagePosts(t, homeFeedPageSize+1), homeFeedPageSize)
	if err != nil {
		t.Fatalf("buildFeedPage: %v", err)
	}
	if len(page) != homeFeedPageSize {
		t.Fatalf("page length = %d, want %d", len(page), homeFeedPageSize)
	}
	if terminated {
		t.Error("terminated = true, want false when an extra row exists")
	}
	c, err := post.DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("DecodeCursor(%q): %v", cursor, err)
	}
	last := page[len(page)-1]
	if c.AfterID != last.ID || !c.Timestamp.Equal(last.CreatedAt) {
		t.Errorf("cursor = (%s, %s), want last returned row (%s, %s)",
			c.AfterID, c.Timestamp, last.ID, last.CreatedAt)
	}
}

// TestBuildFeedPage_NoExtraRow_Terminated verifies that at most pageSize rows
// (window exhausted), including zero rows, yield terminated=true and "".
func TestBuildFeedPage_NoExtraRow_Terminated(t *testing.T) {
	for _, n := range []int{homeFeedPageSize, 1, 0} {
		page, cursor, terminated, err := buildFeedPage(makeFeedPagePosts(t, n), homeFeedPageSize)
		if err != nil {
			t.Fatalf("buildFeedPage(%d rows): %v", n, err)
		}
		if len(page) != n {
			t.Errorf("%d rows: page length = %d", n, len(page))
		}
		if !terminated {
			t.Errorf("%d rows: terminated = false, want true", n)
		}
		if cursor != "" {
			t.Errorf("%d rows: cursor = %q, want empty", n, cursor)
		}
	}
}

// TestHomeFeedDepthConstants guards the approved home feed page size / hard
// depth and the invariant that the depth is a whole number of full pages.
func TestHomeFeedDepthConstants(t *testing.T) {
	if homeFeedPageSize != 50 || maxHomeFeedDepth != 200 {
		t.Errorf("(pageSize, maxDepth) = (%d, %d), want (50, 200)", homeFeedPageSize, maxHomeFeedDepth)
	}
	if maxHomeFeedDepth%homeFeedPageSize != 0 {
		t.Errorf("maxHomeFeedDepth %d must be a multiple of homeFeedPageSize %d", maxHomeFeedDepth, homeFeedPageSize)
	}
}
