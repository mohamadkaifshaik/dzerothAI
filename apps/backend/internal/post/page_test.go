package post

// Unit tests for the pure pagination helpers used by the hard-depth feed
// queries (ListByAuthor, ListThreadReplies, ListByHashtag). The SQL window
// itself is covered by the integration tests in internal/integration.

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func makePagePosts(t *testing.T, n int) []Post {
	t.Helper()
	base := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	posts := make([]Post, n)
	for i := range posts {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatalf("uuid.NewV7: %v", err)
		}
		posts[i] = Post{ID: id, CreatedAt: base.Add(-time.Duration(i) * time.Second)}
	}
	return posts
}

// TestBuildPage_ExtraRow_NotTerminatedWithCursor verifies that pageSize+1 rows
// yield a full page, terminated=false, and a cursor pointing at the last
// returned row.
func TestBuildPage_ExtraRow_NotTerminatedWithCursor(t *testing.T) {
	rows := makePagePosts(t, 51)

	page, cursor, terminated, err := buildPage(rows, 50)
	if err != nil {
		t.Fatalf("buildPage: %v", err)
	}
	if len(page) != 50 {
		t.Fatalf("page length = %d, want 50", len(page))
	}
	if terminated {
		t.Error("terminated = true, want false when an extra row exists")
	}
	if cursor == "" {
		t.Fatal("cursor is empty, want a continuation cursor")
	}
	c, err := DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	last := page[len(page)-1]
	if c.AfterID != last.ID || !c.Timestamp.Equal(last.CreatedAt) {
		t.Errorf("cursor = (%s, %s), want last returned row (%s, %s)",
			c.AfterID, c.Timestamp, last.ID, last.CreatedAt)
	}
}

// TestBuildPage_ExactPageSize_Terminated verifies that exactly pageSize rows
// (no extra row: the window is exhausted) yield terminated=true and "".
func TestBuildPage_ExactPageSize_Terminated(t *testing.T) {
	page, cursor, terminated, err := buildPage(makePagePosts(t, 50), 50)
	if err != nil {
		t.Fatalf("buildPage: %v", err)
	}
	if len(page) != 50 {
		t.Errorf("page length = %d, want 50", len(page))
	}
	if !terminated {
		t.Error("terminated = false, want true when no extra row exists")
	}
	if cursor != "" {
		t.Errorf("cursor = %q, want empty on terminated page", cursor)
	}
}

// TestBuildPage_Empty_Terminated verifies the out-of-window / exhausted case:
// no rows yield an empty, terminated page with an empty cursor.
func TestBuildPage_Empty_Terminated(t *testing.T) {
	page, cursor, terminated, err := buildPage(nil, 25)
	if err != nil {
		t.Fatalf("buildPage: %v", err)
	}
	if len(page) != 0 {
		t.Errorf("page length = %d, want 0", len(page))
	}
	if !terminated {
		t.Error("terminated = false, want true for an empty page")
	}
	if cursor != "" {
		t.Errorf("cursor = %q, want empty", cursor)
	}
}

// TestCursorArgs verifies the nil-cursor → SQL NULL mapping used by the window
// queries, and that a real cursor passes its timestamp and id through.
func TestCursorArgs(t *testing.T) {
	ts, id := cursorArgs(nil)
	if ts != nil || id != nil {
		t.Errorf("cursorArgs(nil) = (%v, %v), want (nil, nil)", ts, id)
	}

	c := &FeedCursor{AfterID: uuid.New(), Timestamp: time.Now().UTC()}
	ts, id = cursorArgs(c)
	if ts != c.Timestamp || id != c.AfterID {
		t.Errorf("cursorArgs(c) = (%v, %v), want (%v, %v)", ts, id, c.Timestamp, c.AfterID)
	}
}

// TestFeedDepthConstants guards the approved page-size / hard-depth pairs and
// the invariant that each depth is a whole number of full pages.
func TestFeedDepthConstants(t *testing.T) {
	cases := []struct {
		name     string
		pageSize int
		maxDepth int
		want     [2]int
	}{
		{"author", pageSizeAuthor, maxDepthAuthor, [2]int{50, 200}},
		{"thread", pageSizeThread, maxDepthThread, [2]int{25, 100}},
		{"hashtag", pageSizeHashtag, maxDepthHashtag, [2]int{50, 200}},
	}
	for _, tc := range cases {
		if tc.pageSize != tc.want[0] || tc.maxDepth != tc.want[1] {
			t.Errorf("%s: (pageSize, maxDepth) = (%d, %d), want (%d, %d)",
				tc.name, tc.pageSize, tc.maxDepth, tc.want[0], tc.want[1])
		}
		if tc.pageSize <= 0 || tc.pageSize >= tc.maxDepth || tc.maxDepth%tc.pageSize != 0 {
			t.Errorf("%s: pageSize %d must be positive, below, and divide maxDepth %d",
				tc.name, tc.pageSize, tc.maxDepth)
		}
	}
}
