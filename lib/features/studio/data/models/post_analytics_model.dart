import '../../domain/entities/post_analytics.dart';

/// JSON data-transfer model for a single post analytics entry.
///
/// Parses the response shape from `GET /api/v1/me/studio/analytics`:
/// ```json
/// {
///   "post_id":        "<uuid>",
///   "content":        "...",
///   "post_type":      "original | reply | quote | repost",
///   "created_at":     "2026-09-18T12:00:00Z",
///   "reaction_count": 7,
///   "bookmark_count": 3,
///   "reply_count":    12,
///   "quote_count":    2
/// }
/// ```
class PostAnalyticsModel {
  const PostAnalyticsModel({
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
  final String postType;
  final DateTime createdAt;
  final int reactionCount;
  final int bookmarkCount;
  final int replyCount;
  final int quoteCount;

  factory PostAnalyticsModel.fromJson(Map<String, dynamic> json) {
    return PostAnalyticsModel(
      postId: json['post_id'] as String,
      content: (json['content'] as String?) ?? '',
      postType: (json['post_type'] as String?) ?? 'original',
      createdAt: DateTime.parse(json['created_at'] as String),
      reactionCount: (json['reaction_count'] as int?) ?? 0,
      bookmarkCount: (json['bookmark_count'] as int?) ?? 0,
      replyCount: (json['reply_count'] as int?) ?? 0,
      quoteCount: (json['quote_count'] as int?) ?? 0,
    );
  }

  /// Maps this model to the domain entity.
  PostAnalytics toEntity() {
    return PostAnalytics(
      postId: postId,
      content: content,
      postType: postType,
      createdAt: createdAt,
      reactionCount: reactionCount,
      bookmarkCount: bookmarkCount,
      replyCount: replyCount,
      quoteCount: quoteCount,
    );
  }
}
