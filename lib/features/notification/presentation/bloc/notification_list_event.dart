part of 'notification_list_bloc.dart';

/// All events that can be dispatched to [NotificationListBloc].
sealed class NotificationListEvent {
  const NotificationListEvent();
}

/// Load the first page of notifications.
final class NotificationListFetchRequested extends NotificationListEvent {
  const NotificationListFetchRequested();
}

/// Load the next page of notifications.
///
/// Ignored when the BLoC is in [NotificationListTerminated] state.
final class NotificationListNextPageRequested extends NotificationListEvent {
  const NotificationListNextPageRequested();
}

/// Mark all notifications as read.
///
/// On success the BLoC re-emits the current loaded/terminated state with all
/// [Notification.isRead] set to true. No re-fetch is performed.
final class NotificationListMarkAllReadRequested extends NotificationListEvent {
  const NotificationListMarkAllReadRequested();
}
