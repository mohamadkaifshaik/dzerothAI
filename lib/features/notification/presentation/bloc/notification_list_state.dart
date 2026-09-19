part of 'notification_list_bloc.dart';

/// All states that [NotificationListBloc] can emit.
sealed class NotificationListState extends Equatable {
  const NotificationListState();

  @override
  List<Object?> get props => [];
}

/// No notifications have been requested yet.
final class NotificationListInitial extends NotificationListState {
  const NotificationListInitial();
}

/// The first page of notifications is loading.
final class NotificationListLoading extends NotificationListState {
  const NotificationListLoading();
}

/// At least one page has been loaded and the list is not yet terminated.
///
/// [hasMore] is false while a next-page request is in-flight, preventing
/// duplicate requests.
final class NotificationListLoaded extends NotificationListState {
  const NotificationListLoaded({
    required this.notifications,
    required this.nextCursor,
    required this.hasMore,
  });

  final List<Notification> notifications;
  final String? nextCursor;

  /// True when [nextCursor] is non-null and no next-page request is in-flight.
  final bool hasMore;

  @override
  List<Object?> get props => [notifications, nextCursor, hasMore];
}

/// The notification list has reached the server-enforced boundary (100 items).
///
/// Per CLAUDE.md §2.1 this is a terminal state. No further pages must be
/// fetched. The UI must show [GoTouchGrassWidget].
final class NotificationListTerminated extends NotificationListState {
  const NotificationListTerminated({required this.notifications});

  final List<Notification> notifications;

  @override
  List<Object?> get props => [notifications];
}

/// No notifications exist for the current user.
final class NotificationListEmpty extends NotificationListState {
  const NotificationListEmpty();
}

/// The notification list request failed.
final class NotificationListError extends NotificationListState {
  const NotificationListError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
