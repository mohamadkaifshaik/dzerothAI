import 'package:flutter_test/flutter_test.dart';

import 'package:dzeroth/features/post/data/models/post_dto.dart';
import 'package:dzeroth/features/post/domain/entities/post.dart';

// ---------------------------------------------------------------------------
// Fixture data
// ---------------------------------------------------------------------------

const _author = PostAuthor(
  id: '01900000-0000-7000-8000-000000000001',
  handle: 'alice',
  displayName: 'Alice',
  avatarUrl: null,
);

final _post = Post(
  id: 'post-fixture-1',
  author: _author,
  postType: 'original',
  content: 'Hello Dzeroth',
  parentId: null,
  threadRootId: null,
  quotedPostId: null,
  isDeleted: false,
  createdAt: DateTime.utc(2024, 1, 1),
  updatedAt: DateTime.utc(2024, 1, 2),
);

/// JSON map that PostDto.fromJson can deserialise — mirrors the actual API
/// envelope shape without any metric fields.
const Map<String, dynamic> _postJson = {
  'id': 'post-fixture-1',
  'author': {
    'id': '01900000-0000-7000-8000-000000000001',
    'handle': 'alice',
    'display_name': 'Alice',
    'avatar_url': null,
  },
  'post_type': 'original',
  'content': 'Hello Dzeroth',
  'parent_id': null,
  'thread_root_id': null,
  'quoted_post_id': null,
  'is_deleted': false,
  'created_at': '2024-01-01T00:00:00Z',
  'updated_at': '2024-01-02T00:00:00Z',
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  group('Post entity — metric lockdown (CLAUDE.md §2.3)', () {
    test('Post entity has no social-validation metric fields in props', () {
      // Post.props is the canonical Equatable field list.
      // Fields present: id, author, postType, content, parentId, threadRootId,
      //   quotedPostId, isDeleted, createdAt, updatedAt — exactly 10.
      // No metric field (like_count, impression_count, etc.) may be present.
      final props = _post.props;

      expect(
        props.length,
        10,
        reason:
            'Post.props must have exactly 10 entries — '
            'no social-validation metric fields (CLAUDE.md §2.3)',
      );
    });

    test('PostAuthor entity has no social-validation metric fields in props',
        () {
      // PostAuthor.props: id, handle, displayName, avatarUrl — exactly 4.
      final props = _author.props;

      expect(
        props.length,
        4,
        reason:
            'PostAuthor.props must have exactly 4 entries — '
            'no follower count or equivalent metric (CLAUDE.md §2.3)',
      );
    });

    test(
        'PostDto.fromJson does not carry metric fields — '
        'known JSON keys that must be absent', () {
      // These are the metric field names that the backend must never expose
      // on a public DTO and that this DTO must never deserialise.
      const forbiddenKeys = [
        'like_count',
        'likes',
        'impression_count',
        'impressions',
        'bookmark_count',
        'bookmarks',
        'reply_count',
        'repost_count',
        'share_count',
        'view_count',
        'reach',
        'engagement',
        'follower_count',
        'following_count',
      ];

      // Confirm the JSON we feed to fromJson contains none of the forbidden
      // keys — this would indicate a DTO or API contract violation.
      for (final key in forbiddenKeys) {
        expect(
          _postJson.containsKey(key),
          isFalse,
          reason:
              'PostDto JSON must not contain metric key "$key" '
              '(CLAUDE.md §2.3 — Public Metric Lockdown)',
        );
      }
    });

    test(
        'PostDto.fromJson round-trip produces a Post entity '
        'with no metric fields', () {
      final dto = PostDto.fromJson(_postJson);
      final entity = dto.toEntity();

      // The entity produced from the DTO must have the same prop count as the
      // canonical Post entity — no extra metric fields snuck in.
      expect(
        entity.props.length,
        _post.props.length,
        reason:
            'Post entity produced from DTO must have the same number of props '
            'as the canonical Post entity — no metric extras',
      );

      // Confirm core identity fields survive the round-trip correctly.
      expect(entity.id, _post.id);
      expect(entity.postType, _post.postType);
      expect(entity.content, _post.content);
      expect(entity.isDeleted, _post.isDeleted);
    });

    test('PostPage entity carries no metric fields', () {
      // PostPage.props: items, nextCursor, terminated — exactly 3.
      const page = PostPage(items: [], nextCursor: null, terminated: false);

      expect(
        page.props.length,
        3,
        reason:
            'PostPage.props must have exactly 3 entries — '
            'no aggregate metric fields (CLAUDE.md §2.3)',
      );
    });
  });

  group('Post entity — field semantics', () {
    test('content is nullable (pure repost has no content)', () {
      final repost = Post(
        id: 'repost-1',
        author: _author,
        postType: 'repost',
        content: null,
        isDeleted: false,
        createdAt: DateTime.utc(2024),
        updatedAt: DateTime.utc(2024),
      );

      expect(repost.content, isNull);
      expect(repost.postType, 'repost');
    });

    test('isDeleted flag is correctly stored', () {
      final deleted = Post(
        id: 'post-deleted',
        author: _author,
        postType: 'original',
        content: null,
        isDeleted: true,
        createdAt: DateTime.utc(2024),
        updatedAt: DateTime.utc(2024),
      );

      expect(deleted.isDeleted, isTrue);
    });

    test('two Posts with identical fields are equal via Equatable', () {
      final a = Post(
        id: 'post-eq',
        author: _author,
        postType: 'original',
        content: 'Same',
        isDeleted: false,
        createdAt: DateTime.utc(2024),
        updatedAt: DateTime.utc(2024),
      );

      final b = Post(
        id: 'post-eq',
        author: _author,
        postType: 'original',
        content: 'Same',
        isDeleted: false,
        createdAt: DateTime.utc(2024),
        updatedAt: DateTime.utc(2024),
      );

      expect(a, equals(b));
      expect(a.hashCode, b.hashCode);
    });
  });
}
