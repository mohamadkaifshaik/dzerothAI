import '../../domain/entities/notification.dart';

/// JSON DTO for a single actor summary embedded in a notification response.
class ActorSummaryModel {
  const ActorSummaryModel({
    required this.id,
    required this.handle,
    required this.displayName,
    this.avatarUrl,
  });

  final String id;
  final String handle;
  final String displayName;
  final String? avatarUrl;

  factory ActorSummaryModel.fromJson(Map<String, dynamic> json) {
    return ActorSummaryModel(
      id: json['id'] as String,
      handle: json['handle'] as String,
      displayName: json['display_name'] as String,
      avatarUrl: json['avatar_url'] as String?,
    );
  }

  ActorSummary toEntity() => ActorSummary(
    id: id,
    handle: handle,
    displayName: displayName,
    avatarUrl: avatarUrl,
  );
}

/// JSON DTO for a single notification item.
///
/// Maps the backend response shape:
/// ```json
/// {
///   "id": "<uuid-v7>",
///   "event": "follow | mention | reply | reaction",
///   "actor": { "id": "...", "handle": "...", "display_name": "...", "avatar_url": null },
///   "post_id": "<uuid-v7 | null>",
///   "is_read": false,
///   "created_at": "2026-09-18T12:00:00Z"
/// }
/// ```
class NotificationModel {
  const NotificationModel({
    required this.id,
    required this.event,
    required this.actor,
    this.postId,
    required this.isRead,
    required this.createdAt,
  });

  final String id;
  final String event;
  final ActorSummaryModel actor;
  final String? postId;
  final bool isRead;
  final DateTime createdAt;

  factory NotificationModel.fromJson(Map<String, dynamic> json) {
    return NotificationModel(
      id: json['id'] as String,
      event: json['event'] as String,
      actor: ActorSummaryModel.fromJson(json['actor'] as Map<String, dynamic>),
      postId: json['post_id'] as String?,
      isRead: (json['is_read'] as bool?) ?? false,
      createdAt: DateTime.parse(json['created_at'] as String),
    );
  }

  Notification toEntity() => Notification(
    id: id,
    event: NotificationEvent.fromString(event),
    actor: actor.toEntity(),
    postId: postId,
    isRead: isRead,
    createdAt: createdAt,
  );
}
