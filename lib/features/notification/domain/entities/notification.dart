import 'package:equatable/equatable.dart';

/// The type of event that triggered a notification.
enum NotificationEvent {
  follow,
  mention,
  reply,
  reaction;

  /// Parses the backend ENUM string value.
  static NotificationEvent fromString(String value) {
    return switch (value) {
      'follow' => follow,
      'mention' => mention,
      'reply' => reply,
      'reaction' => reaction,
      _ => reaction, // safe default for unknown future values
    };
  }

  /// Human-readable label shown in the notification list.
  String get label => switch (this) {
    follow => 'followed you',
    mention => 'mentioned you',
    reply => 'replied to your post',
    reaction => 'reacted to your post',
  };
}

/// A summary of the actor who triggered the notification.
class ActorSummary extends Equatable {
  const ActorSummary({
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

/// A single notification entry belonging to the authenticated user.
///
/// PUBLIC METRICS LOCKDOWN: no reaction counts, bookmark counts, follower
/// counts, or impression metrics are stored or exposed here (CLAUDE.md §2.3).
class Notification extends Equatable {
  const Notification({
    required this.id,
    required this.event,
    required this.actor,
    this.postId,
    required this.isRead,
    required this.createdAt,
  });

  final String id;
  final NotificationEvent event;
  final ActorSummary actor;

  /// Non-null for events tied to a post (mention, reply, reaction).
  final String? postId;

  final bool isRead;
  final DateTime createdAt;

  /// Returns a copy of this notification with [isRead] set to true.
  Notification markRead() => Notification(
    id: id,
    event: event,
    actor: actor,
    postId: postId,
    isRead: true,
    createdAt: createdAt,
  );

  @override
  List<Object?> get props => [id, event, actor, postId, isRead, createdAt];
}

/// A paginated page of [Notification] entries.
///
/// [terminated] is true when the backend has reached the configured boundary
/// (100 items). Per CLAUDE.md §2.1 the client must stop fetching and show the
/// termination experience when this is true.
class NotificationPage extends Equatable {
  const NotificationPage({
    required this.items,
    this.nextCursor,
    required this.terminated,
  });

  final List<Notification> items;

  /// Opaque cursor for the next page. Null when no further pages exist.
  final String? nextCursor;

  /// True when the server-enforced boundary has been reached.
  final bool terminated;

  @override
  List<Object?> get props => [items, nextCursor, terminated];
}
