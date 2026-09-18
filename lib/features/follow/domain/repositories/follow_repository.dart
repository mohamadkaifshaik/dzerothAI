import '../../../../core/error/result.dart';
import '../entities/follow_user.dart';
import '../entities/follow_page.dart';

/// Contract for follow/unfollow and follow-list operations.
///
/// PUBLIC METRICS LOCKDOWN: [FollowUser] deliberately carries no
/// follower/following count fields per CLAUDE.md §2.3.
abstract class FollowRepository {
  /// Follow the user identified by [targetUserId].
  Future<Result<void>> follow(String targetUserId);

  /// Unfollow the user identified by [targetUserId].
  Future<Result<void>> unfollow(String targetUserId);

  /// Return true when the authenticated caller follows [targetUserId].
  Future<Result<bool>> isFollowing(String targetUserId);

  /// List users that [userId] follows, paginated by [cursor].
  Future<Result<FollowPage>> listFollowing(String userId, {String? cursor});

  /// List followers of [userId], paginated by [cursor].
  Future<Result<FollowPage>> listFollowers(String userId, {String? cursor});
}
