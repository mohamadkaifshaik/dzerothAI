import 'package:equatable/equatable.dart';

/// Author information embedded in a post.
///
/// Carries only display fields. No social-validation metrics are included.
class PostAuthor extends Equatable {
  const PostAuthor({
    required this.id,
    required this.handle,
    required this.displayName,
    this.avatarUrl,
  });

  final String id;
  final String handle;
  final String displayName;
  final String? avatarUrl;

  @override
  List<Object?> get props => [id, handle, displayName, avatarUrl];
}

/// A single Dzeroth post.
///
/// IMPORTANT: This entity intentionally contains zero social-validation metric
/// fields (likes, impressions, bookmarks, reply counts, repost counts, view
/// counts, share counts, follower counts, or any equivalent popularity metric)
/// per CLAUDE.md §2.3.  These values must never be added here or rendered in
/// any public feed or post-detail UI layer.
class Post extends Equatable {
  const Post({
    required this.id,
    required this.author,
    required this.postType,
    this.content,
    this.parentId,
    this.threadRootId,
    this.quotedPostId,
    required this.isDeleted,
    required this.createdAt,
    required this.updatedAt,
  });

  final String id;
  final PostAuthor author;

  /// One of: 'original', 'reply', 'quote', 'repost'.
  final String postType;

  /// Null only for pure reposts.
  final String? content;

  final String? parentId;
  final String? threadRootId;
  final String? quotedPostId;
  final bool isDeleted;
  final DateTime createdAt;
  final DateTime updatedAt;

  @override
  List<Object?> get props => [
    id,
    author,
    postType,
    content,
    parentId,
    threadRootId,
    quotedPostId,
    isDeleted,
    createdAt,
    updatedAt,
  ];
}

/// A paginated page of posts.
///
/// [terminated] is true when the backend has reached the configured feed
/// boundary.  Per CLAUDE.md §2.1 the client must stop fetching and display
/// the termination experience when this is true.
class PostPage extends Equatable {
  const PostPage({
    required this.items,
    this.nextCursor,
    required this.terminated,
  });

  final List<Post> items;

  /// Opaque cursor for the next page.  Null when there are no further pages.
  final String? nextCursor;

  /// True when the server-enforced feed boundary has been reached.
  /// The client must enter [PostFeedTerminated] state and never fetch again.
  final bool terminated;

  @override
  List<Object?> get props => [items, nextCursor, terminated];
}
