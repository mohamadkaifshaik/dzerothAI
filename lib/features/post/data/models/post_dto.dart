import '../../domain/entities/post.dart';

/// DTO for the author sub-object returned inside every PostDTO.
///
/// Field names match the Go [PostAuthor] struct JSON tags.
class PostAuthorDto {
  const PostAuthorDto({
    required this.id,
    required this.handle,
    required this.displayName,
    this.avatarUrl,
  });

  final String id;
  final String handle;
  final String displayName;
  final String? avatarUrl;

  factory PostAuthorDto.fromJson(Map<String, dynamic> json) {
    return PostAuthorDto(
      id: json['ID'] as String? ?? json['id'] as String,
      handle: json['Handle'] as String? ?? json['handle'] as String,
      displayName:
          json['DisplayName'] as String? ?? json['display_name'] as String,
      avatarUrl: json['AvatarURL'] as String? ?? json['avatar_url'] as String?,
    );
  }

  PostAuthor toEntity() => PostAuthor(
    id: id,
    handle: handle,
    displayName: displayName,
    avatarUrl: avatarUrl,
  );
}

/// DTO for a single post (matches Go PostDTO struct).
///
/// IMPORTANT: This DTO intentionally contains zero social-validation metric
/// fields per CLAUDE.md §2.3.  Do not add like_count, impression_count,
/// bookmark_count, reply_count, repost_count, view_count, share_count, or
/// any equivalent field.  If the backend ever returns such a field it is
/// silently ignored here.
class PostDto {
  const PostDto({
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
  final PostAuthorDto author;
  final String postType;
  final String? content;
  final String? parentId;
  final String? threadRootId;
  final String? quotedPostId;
  final bool isDeleted;
  final String createdAt;
  final String updatedAt;

  factory PostDto.fromJson(Map<String, dynamic> json) {
    final authorRaw = json['author'];
    final PostAuthorDto author;
    if (authorRaw is Map<String, dynamic>) {
      author = PostAuthorDto.fromJson(authorRaw);
    } else {
      // Fallback: construct a minimal author from top-level fields.
      author = PostAuthorDto(
        id: json['author_id'] as String? ?? '',
        handle: '',
        displayName: '',
      );
    }

    return PostDto(
      id: json['id'] as String,
      author: author,
      postType: json['post_type'] as String,
      content: json['content'] as String?,
      parentId: json['parent_id'] as String?,
      threadRootId: json['thread_root_id'] as String?,
      quotedPostId: json['quoted_post_id'] as String?,
      isDeleted: (json['is_deleted'] as bool?) ?? false,
      createdAt: json['created_at'] as String,
      updatedAt: json['updated_at'] as String,
    );
  }

  Post toEntity() => Post(
    id: id,
    author: author.toEntity(),
    postType: postType,
    content: content,
    parentId: parentId,
    threadRootId: threadRootId,
    quotedPostId: quotedPostId,
    isDeleted: isDeleted,
    createdAt: DateTime.parse(createdAt),
    updatedAt: DateTime.parse(updatedAt),
  );
}

/// DTO for the paginated feed envelope.
///
/// Corresponds to the Go [PostPage] struct.  [terminated] MUST propagate to
/// the feed BLoC so it can enter [PostFeedTerminated] and stop fetching.
class PostPageDto {
  const PostPageDto({
    required this.items,
    required this.nextCursor,
    required this.terminated,
  });

  final List<PostDto> items;
  final String nextCursor;
  final bool terminated;

  factory PostPageDto.fromJson(Map<String, dynamic> json) {
    final rawItems = json['items'];
    final items = rawItems is List
        ? rawItems
              .whereType<Map<String, dynamic>>()
              .map(PostDto.fromJson)
              .toList()
        : <PostDto>[];

    return PostPageDto(
      items: items,
      nextCursor: (json['next_cursor'] as String?) ?? '',
      terminated: (json['terminated'] as bool?) ?? false,
    );
  }
}
