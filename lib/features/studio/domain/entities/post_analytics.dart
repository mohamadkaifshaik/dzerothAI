import 'package:equatable/equatable.dart';

/// Private analytics data for a single post owned by the authenticated user.
///
/// PRIVATE DATA — these counts must ONLY be displayed inside [StudioScreen]
/// (the authenticated author's private Creator Studio). They must NOT appear
/// in any public feed, profile page, or search result per CLAUDE.md §2.3.
class PostAnalytics extends Equatable {
  const PostAnalytics({
    required this.postId,
    required this.content,
    required this.postType,
    required this.createdAt,
    required this.reactionCount,
    required this.bookmarkCount,
    required this.replyCount,
    required this.quoteCount,
  });

  final String postId;
  final String content;

  /// One of: "original", "reply", "quote", "repost".
  final String postType;

  final DateTime createdAt;

  // --- Private engagement counts ---
  // These fields are intentionally restricted to the private Creator Studio.
  // Do not pass these to any public-facing widget or DTO.

  final int reactionCount;
  final int bookmarkCount;
  final int replyCount;
  final int quoteCount;

  @override
  List<Object?> get props => [
    postId,
    content,
    postType,
    createdAt,
    reactionCount,
    bookmarkCount,
    replyCount,
    quoteCount,
  ];
}
