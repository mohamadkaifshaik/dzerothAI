import '../../../../core/error/result.dart';
import '../entities/notification.dart';

/// Contract for notification list and mark-read operations.
///
/// All operations are scoped to the authenticated user via JWT — no user
/// ID is present in any request path.
///
/// PUBLIC METRICS LOCKDOWN: No reaction counts, bookmark counts, follower
/// counts, or impression metrics are ever returned (CLAUDE.md §2.3).
abstract class NotificationRepository {
  /// Returns a cursor-paginated page of notifications for the current user.
  ///
  /// Pass [cursor] to fetch subsequent pages. The backend terminates at
  /// 100 items; when [NotificationPage.terminated] is true the client must
  /// stop fetching and display the termination experience.
  Future<Result<NotificationPage>> getNotifications({String? cursor});

  /// Marks all of the current user's notifications as read.
  ///
  /// Returns [Result<void>] — the caller should re-render existing items
  /// with [isRead] == true rather than re-fetching the page.
  Future<Result<void>> markAllRead();
}
