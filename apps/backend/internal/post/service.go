package post

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
)

const (
	maxPostRunes   = 500
	maxHashtags    = 10
	maxDepthAuthor = 200
	maxDepthThread = 100

	// minQuoteWords is the minimum number of distinct words required in quote
	// post content. This is a Dzeroth product invariant (CLAUDE.md §2.2).
	// The normalization strategy here (lowercase, strip leading/trailing
	// non-alphanumeric) must match the Flutter client implementation.
	minQuoteWords = 5
)

// hashtagPattern matches #word tokens where word begins with a letter and
// contains only alphanumeric characters and underscores.
var hashtagPattern = regexp.MustCompile(`#([a-zA-Z][a-zA-Z0-9_]*)`)

// mentionPattern matches @handle tokens.
// Handle characters: letters, digits, underscores, hyphens.
var mentionPattern = regexp.MustCompile(`@([a-zA-Z0-9][a-zA-Z0-9_-]{1,49})`)

// FollowChecker allows post.Service to check follower relationships without
// importing the follow package directly, avoiding a circular dependency.
// follow.Service satisfies this interface via its IsFollowing method.
type FollowChecker interface {
	IsFollowing(ctx context.Context, followerID, followedID uuid.UUID) (bool, error)
}

// PostNotificationEvent carries the fields needed to publish a single
// notification from post.Service. It is defined here to avoid a circular
// import between internal/post and internal/notification.
// The Event string values must match those in internal/notification/model.go.
type PostNotificationEvent struct {
	RecipientID uuid.UUID
	ActorID     uuid.UUID
	Event       string
	PostID      *uuid.UUID
}

// PostNotificationPublisher is the interface used by post.Service to emit
// notification events without importing internal/notification directly
// (which would create an import cycle since notification imports post.FeedCursor).
// An adapter in cmd/api/main.go bridges this interface to notification.Service.
type PostNotificationPublisher interface {
	PublishPostEvent(ctx context.Context, event PostNotificationEvent) error
}

// Service implements post business logic.
type Service struct {
	repo          *Repository
	log           *zap.Logger
	followChecker FollowChecker
	notifier      PostNotificationPublisher
}

// NewService constructs a post Service.
func NewService(repo *Repository, log *zap.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// SetFollowChecker injects a FollowChecker dependency. Must be called after
// NewService and before the first request is served. Concurrency-safe if
// called during the single-threaded startup phase before the HTTP server starts.
func (s *Service) SetFollowChecker(fc FollowChecker) {
	s.followChecker = fc
}

// SetNotificationPublisher injects a PostNotificationPublisher dependency. Must be
// called after NewService and before the first request is served. Concurrency-safe
// if called during the single-threaded startup phase before the HTTP server starts.
func (s *Service) SetNotificationPublisher(np PostNotificationPublisher) {
	s.notifier = np
}

// PostExistsAndNotDeleted returns true when the post exists and has not been
// soft-deleted. Satisfies the report.PostChecker interface.
// Any repository error is treated as "not found" (returns false, nil) so that
// the caller receives a clean not-found signal rather than an internal error.
func (s *Service) PostExistsAndNotDeleted(ctx context.Context, postID uuid.UUID) (bool, error) {
	p, err := s.repo.GetByID(ctx, postID)
	if err != nil {
		return false, nil
	}
	return !p.IsDeleted, nil
}

// GetPostAuthorID returns the AuthorID of the post identified by postID.
// Returns a CodeNotFound error if the post does not exist or has been soft-deleted.
// This method satisfies the reaction.PostAuthorLookup interface.
func (s *Service) GetPostAuthorID(ctx context.Context, postID uuid.UUID) (uuid.UUID, error) {
	p, err := s.repo.GetByID(ctx, postID)
	if err != nil {
		if err == ErrNotFound {
			return uuid.UUID{}, apierror.NewAPIError(apierror.CodeNotFound, "post not found")
		}
		s.log.Error("post: get post author id", zap.Error(err))
		return uuid.UUID{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	if p.IsDeleted {
		return uuid.UUID{}, apierror.NewAPIError(apierror.CodeNotFound, "post not found")
	}
	return p.AuthorID, nil
}

// CreatePost validates the request, extracts mentions and hashtags, generates a
// UUID v7, and persists the post and its associations in a single transaction.
func (s *Service) CreatePost(ctx context.Context, authorID uuid.UUID, req CreatePostRequest) (PostDTO, error) {
	// Validate post type.
	switch req.PostType {
	case PostTypeOriginal, PostTypeReply, PostTypeQuote, PostTypeRepost:
		// valid
	default:
		if req.PostType == "" {
			req.PostType = PostTypeOriginal
		} else {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, "invalid post_type")
		}
	}

	// Content validation — reposts carry no authored text.
	if req.PostType != PostTypeRepost {
		count := utf8.RuneCountInString(req.Content)
		if count == 0 {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, "content is required")
		}
		if count > maxPostRunes {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, fmt.Sprintf("content exceeds %d code points", maxPostRunes))
		}
	}

	// Validate reply requires parent_id.
	if req.PostType == PostTypeReply && req.ParentID == nil {
		return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, "parent_id is required for reply posts")
	}

	// Validate quote and repost require quoted_post_id.
	if (req.PostType == PostTypeQuote || req.PostType == PostTypeRepost) && req.QuotedPostID == nil {
		return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, "quoted_post_id is required for quote and repost posts")
	}

	// Quote posts require at least 5 distinct words in the content.
	// This is a Dzeroth product invariant (CLAUDE.md §2.2). The backend is the
	// authoritative enforcement point; the client must apply the same check
	// before submission but cannot be trusted to enforce it alone.
	if req.PostType == PostTypeQuote {
		if countDistinctWords(req.Content) < minQuoteWords {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation,
				fmt.Sprintf("quote post content must contain at least %d distinct words", minQuoteWords))
		}
	}

	// Parse optional UUID string fields.
	var parentID *uuid.UUID
	if req.ParentID != nil {
		parsed, err := uuid.Parse(*req.ParentID)
		if err != nil {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, "parent_id must be a valid UUID")
		}
		parentID = &parsed
	}

	var quotedPostID *uuid.UUID
	if req.QuotedPostID != nil {
		parsed, err := uuid.Parse(*req.QuotedPostID)
		if err != nil {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, "quoted_post_id must be a valid UUID")
		}
		quotedPostID = &parsed
	}

	// Determine thread_root_id: for replies, inherit from the parent's thread root
	// if the parent is itself a reply, else use the parent_id as the root.
	// For non-replies this is nil.
	var threadRootID *uuid.UUID
	if req.PostType == PostTypeReply && parentID != nil {
		parent, err := s.repo.GetByID(ctx, *parentID)
		if err != nil {
			if err == ErrNotFound {
				return PostDTO{}, apierror.NewAPIError(apierror.CodeNotFound, "parent post not found")
			}
			s.log.Error("post: get parent for thread root", zap.Error(err))
			return PostDTO{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
		}
		if parent.ThreadRootID != nil {
			threadRootID = parent.ThreadRootID
		} else {
			threadRootID = parentID
		}
	}

	// Extract hashtags and mentions from content.
	var hashtags []string
	var mentionUserIDs []uuid.UUID

	if req.PostType != PostTypeRepost {
		hashtags = extractHashtags(req.Content)
		handles := extractMentions(req.Content)
		for _, handle := range handles {
			userID, err := s.repo.GetAuthorHandleByHandle(ctx, handle)
			if err != nil {
				if err == ErrNotFound {
					// Unknown handle — skip without error.
					continue
				}
				s.log.Warn("post: mention handle lookup failed", zap.String("handle", handle), zap.Error(err))
				continue
			}
			mentionUserIDs = append(mentionUserIDs, userID)
		}
	}

	// Generate post ID.
	postID, err := uuid.NewV7()
	if err != nil {
		return PostDTO{}, fmt.Errorf("post: generate id: %w", err)
	}

	now := time.Now().UTC()
	var contentPtr *string
	if req.PostType != PostTypeRepost {
		c := req.Content
		contentPtr = &c
	}

	p := Post{
		ID:           postID,
		AuthorID:     authorID,
		PostType:     req.PostType,
		Content:      contentPtr,
		ParentID:     parentID,
		ThreadRootID: threadRootID,
		QuotedPostID: quotedPostID,
		IsDeleted:    false,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.Create(ctx, p, mentionUserIDs, hashtags); err != nil {
		s.log.Error("post: create", zap.Error(err))
		return PostDTO{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	// Fetch the created post with the author join to return a fully populated DTO.
	created, err := s.repo.GetByID(ctx, postID)
	if err != nil {
		s.log.Error("post: fetch after create", zap.Error(err))
		return PostDTO{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	// Publish best-effort notifications. Notification failures must not fail
	// the primary create operation (CLAUDE.md §2, §12 — reliability).
	if s.notifier != nil {
		// Reply notification: notify the parent post's author.
		if req.PostType == PostTypeReply && parentID != nil {
			parent, parentErr := s.repo.GetByID(ctx, *parentID)
			if parentErr == nil && parent.AuthorID != authorID {
				postIDCopy := postID
				if pubErr := s.notifier.PublishPostEvent(ctx, PostNotificationEvent{
					RecipientID: parent.AuthorID,
					ActorID:     authorID,
					Event:       "reply",
					PostID:      &postIDCopy,
				}); pubErr != nil {
					s.log.Warn("post: publish reply notification failed",
						zap.String("post_id", postID.String()),
						zap.Error(pubErr),
					)
				}
			}
		}

		// Mention notifications: notify each mentioned user.
		for _, mentionedID := range mentionUserIDs {
			if mentionedID == authorID {
				continue
			}
			mentionedIDCopy := mentionedID
			postIDCopy := postID
			if pubErr := s.notifier.PublishPostEvent(ctx, PostNotificationEvent{
				RecipientID: mentionedIDCopy,
				ActorID:     authorID,
				Event:       "mention",
				PostID:      &postIDCopy,
			}); pubErr != nil {
				s.log.Warn("post: publish mention notification failed",
					zap.String("post_id", postID.String()),
					zap.String("mentioned_id", mentionedIDCopy.String()),
					zap.Error(pubErr),
				)
			}
		}
	}

	return ToDTO(created), nil
}

// GetPost returns the PostDTO for a single post by ID.
// Returns a CodeNotFound error if the post does not exist.
func (s *Service) GetPost(ctx context.Context, postID uuid.UUID) (PostDTO, error) {
	p, err := s.repo.GetByID(ctx, postID)
	if err != nil {
		if err == ErrNotFound {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeNotFound, "post not found")
		}
		s.log.Error("post: get post", zap.Error(err))
		return PostDTO{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return ToDTO(p), nil
}

// ListPostsByAuthor returns a paginated PostPage for the given author.
// The server-enforced maximum depth is 200 posts (CLAUDE.md §2.1, no infinite scrolling).
// When the author's account is private, posts are returned only to the account
// owner or an approved follower. All other callers receive an empty terminated
// page. The follow check requires SetFollowChecker to have been called; when no
// follow checker is configured, non-owner callers are denied as a safe default.
func (s *Service) ListPostsByAuthor(ctx context.Context, callerID *uuid.UUID, authorID uuid.UUID, cursorStr string) (PostPage, error) {
	// Private account check.
	isPrivate, err := s.repo.GetAuthorIsPrivate(ctx, authorID)
	if err != nil {
		if err == ErrNotFound {
			return PostPage{}, apierror.NewAPIError(apierror.CodeNotFound, "user not found")
		}
		s.log.Error("post: list by author privacy check", zap.Error(err))
		return PostPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	if isPrivate {
		isOwner := callerID != nil && *callerID == authorID
		if !isOwner {
			// Phase-3: check follower relationship. Unauthenticated callers (nil
			// callerID) and authenticated callers who are not approved followers
			// both receive an empty terminated page — the private account's posts
			// are not revealed.
			if callerID == nil || s.followChecker == nil {
				return PostPage{Items: []PostDTO{}, NextCursor: "", Terminated: true}, nil
			}
			isFollowing, err := s.followChecker.IsFollowing(ctx, *callerID, authorID)
			if err != nil {
				s.log.Error("post: list by author follow check", zap.Error(err))
				return PostPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
			}
			if !isFollowing {
				return PostPage{Items: []PostDTO{}, NextCursor: "", Terminated: true}, nil
			}
			// Caller is an approved follower — fall through to fetch posts.
		}
	}

	var cursor *FeedCursor
	if cursorStr != "" {
		c, err := DecodeCursor(cursorStr)
		if err != nil {
			return PostPage{}, apierror.NewAPIError(apierror.CodeValidation, "invalid cursor")
		}
		cursor = c
	}

	posts, nextCursor, terminated, err := s.repo.ListByAuthor(ctx, authorID, cursor, maxDepthAuthor)
	if err != nil {
		s.log.Error("post: list by author", zap.Error(err))
		return PostPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	dtos := make([]PostDTO, len(posts))
	for i, p := range posts {
		dtos[i] = ToDTO(p)
	}

	return PostPage{Items: dtos, NextCursor: nextCursor, Terminated: terminated}, nil
}

// ListThreadReplies returns a paginated PostPage of replies in the given thread.
// The server-enforced maximum depth is 100 replies (CLAUDE.md §2.1, no infinite scrolling).
func (s *Service) ListThreadReplies(ctx context.Context, threadRootID uuid.UUID, cursorStr string) (PostPage, error) {
	var cursor *FeedCursor
	if cursorStr != "" {
		c, err := DecodeCursor(cursorStr)
		if err != nil {
			return PostPage{}, apierror.NewAPIError(apierror.CodeValidation, "invalid cursor")
		}
		cursor = c
	}

	posts, nextCursor, terminated, err := s.repo.ListThreadReplies(ctx, threadRootID, cursor, maxDepthThread)
	if err != nil {
		s.log.Error("post: list thread replies", zap.Error(err))
		return PostPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	dtos := make([]PostDTO, len(posts))
	for i, p := range posts {
		dtos[i] = ToDTO(p)
	}

	return PostPage{Items: dtos, NextCursor: nextCursor, Terminated: terminated}, nil
}

// DeletePost soft-deletes the post. Verifies ownership before acting.
// Returns CodeForbidden if the caller is not the post author.
// Returns CodeNotFound if the post does not exist.
func (s *Service) DeletePost(ctx context.Context, callerID uuid.UUID, postID uuid.UUID) error {
	// Verify the post exists and the caller is the owner.
	p, err := s.repo.GetByID(ctx, postID)
	if err != nil {
		if err == ErrNotFound {
			return apierror.NewAPIError(apierror.CodeNotFound, "post not found")
		}
		s.log.Error("post: delete get by id", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	if p.AuthorID != callerID {
		return apierror.NewAPIError(apierror.CodeForbidden, "you may not delete another user's post")
	}

	if err := s.repo.SoftDelete(ctx, postID, callerID); err != nil {
		if err == ErrNotFound {
			return apierror.NewAPIError(apierror.CodeNotFound, "post not found")
		}
		s.log.Error("post: soft delete", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	return nil
}

// extractHashtags scans content for #word tokens, normalizes to lowercase,
// deduplicates, and returns at most maxHashtags values. The leading '#' is
// stripped before return.
func extractHashtags(content string) []string {
	matches := hashtagPattern.FindAllStringSubmatch(content, -1)
	seen := make(map[string]struct{}, len(matches))
	var tags []string
	for _, m := range matches {
		tag := strings.ToLower(m[1])
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
		if len(tags) >= maxHashtags {
			break
		}
	}
	return tags
}

// countDistinctWords counts the number of distinct normalized words in content.
// Normalization: split on whitespace, lowercase each token, strip leading and
// trailing non-letter/non-digit runes (punctuation, symbols). Tokens that are
// empty after stripping are ignored.
//
// This normalization strategy is the authoritative backend definition and must
// match the Flutter client's implementation (CLAUDE.md §2.2).
func countDistinctWords(content string) int {
	tokens := strings.Fields(content)
	seen := make(map[string]struct{}, len(tokens))
	for _, tok := range tokens {
		// Strip leading and trailing non-alphanumeric runes.
		word := strings.TrimFunc(strings.ToLower(tok), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if word == "" {
			continue
		}
		seen[word] = struct{}{}
	}
	return len(seen)
}

// extractMentions scans content for @handle tokens and returns the unique
// set of handles found (without the leading '@').
func extractMentions(content string) []string {
	matches := mentionPattern.FindAllStringSubmatch(content, -1)
	seen := make(map[string]struct{}, len(matches))
	var handles []string
	for _, m := range matches {
		handle := m[1]
		lower := strings.ToLower(handle)
		if _, exists := seen[lower]; exists {
			continue
		}
		seen[lower] = struct{}{}
		handles = append(handles, handle)
	}
	return handles
}
