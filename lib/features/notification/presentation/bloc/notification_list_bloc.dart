import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/entities/notification.dart';
import '../../domain/repositories/notification_repository.dart';

part 'notification_list_event.dart';
part 'notification_list_state.dart';

/// Manages the paginated notification list for the authenticated user.
///
/// Per CLAUDE.md §2.1 there is NO infinite scrolling.
/// When the repository returns [NotificationPage.terminated] == true, this BLoC
/// emits [NotificationListTerminated] and silently ignores all subsequent
/// [NotificationListNextPageRequested] events.
///
/// The backend terminates at 100 items.
///
/// Mark-all-read does NOT re-fetch from the server — it re-emits the current
/// loaded state with all [Notification.isRead] set to true, keeping the UX
/// responsive. The server is authoritative; the client only mutates local state
/// after a successful PUT.
class NotificationListBloc
    extends Bloc<NotificationListEvent, NotificationListState> {
  NotificationListBloc({required NotificationRepository notificationRepository})
    : _repository = notificationRepository,
      super(const NotificationListInitial()) {
    on<NotificationListFetchRequested>(_onFetchRequested);
    on<NotificationListNextPageRequested>(_onNextPageRequested);
    on<NotificationListMarkAllReadRequested>(_onMarkAllReadRequested);
  }

  final NotificationRepository _repository;

  Future<void> _onFetchRequested(
    NotificationListFetchRequested event,
    Emitter<NotificationListState> emit,
  ) async {
    emit(const NotificationListLoading());

    final result = await _repository.getNotifications(cursor: null);

    switch (result) {
      case Success(:final value):
        if (value.items.isEmpty && !value.terminated) {
          emit(const NotificationListEmpty());
        } else if (value.terminated) {
          if (value.items.isEmpty) {
            emit(const NotificationListEmpty());
          } else {
            emit(NotificationListTerminated(notifications: value.items));
          }
        } else {
          emit(
            NotificationListLoaded(
              notifications: value.items,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
            ),
          );
        }
      case Err(:final failure):
        emit(NotificationListError(failure: failure));
    }
  }

  Future<void> _onNextPageRequested(
    NotificationListNextPageRequested event,
    Emitter<NotificationListState> emit,
  ) async {
    // Critical invariant: once terminated, reject all further page requests.
    final current = state;
    if (current is NotificationListTerminated) return;
    if (current is! NotificationListLoaded) return;
    if (!current.hasMore) return;

    // Lock hasMore to false while in-flight to prevent duplicate requests.
    emit(
      NotificationListLoaded(
        notifications: current.notifications,
        nextCursor: current.nextCursor,
        hasMore: false,
      ),
    );

    final result = await _repository.getNotifications(
      cursor: current.nextCursor,
    );

    switch (result) {
      case Success(:final value):
        final merged = [...current.notifications, ...value.items];
        if (value.terminated) {
          emit(NotificationListTerminated(notifications: merged));
        } else {
          emit(
            NotificationListLoaded(
              notifications: merged,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
            ),
          );
        }
      case Err(:final failure):
        // Restore previous loaded state so the user can retry.
        emit(
          NotificationListLoaded(
            notifications: current.notifications,
            nextCursor: current.nextCursor,
            hasMore: current.nextCursor != null,
          ),
        );
        emit(NotificationListError(failure: failure));
    }
  }

  Future<void> _onMarkAllReadRequested(
    NotificationListMarkAllReadRequested event,
    Emitter<NotificationListState> emit,
  ) async {
    final current = state;

    // Only act when there are loaded notifications.
    if (current is! NotificationListLoaded &&
        current is! NotificationListTerminated) {
      return;
    }

    final result = await _repository.markAllRead();

    switch (result) {
      case Success():
        // Re-emit current state with all notifications marked as read.
        // No re-fetch — the local mutation is sufficient for immediate feedback.
        if (current is NotificationListLoaded) {
          emit(
            NotificationListLoaded(
              notifications: current.notifications
                  .map((n) => n.markRead())
                  .toList(),
              nextCursor: current.nextCursor,
              hasMore: current.hasMore,
            ),
          );
        } else if (current is NotificationListTerminated) {
          emit(
            NotificationListTerminated(
              notifications: current.notifications
                  .map((n) => n.markRead())
                  .toList(),
            ),
          );
        }
      case Err(:final failure):
        emit(NotificationListError(failure: failure));
    }
  }
}
