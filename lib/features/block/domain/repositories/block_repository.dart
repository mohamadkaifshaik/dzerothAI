import '../../../../core/error/result.dart';

/// Contract for block and mute operations.
abstract class BlockRepository {
  /// Block the user identified by [targetUserId].
  Future<Result<void>> blockUser(String targetUserId);

  /// Unblock the user identified by [targetUserId].
  Future<Result<void>> unblockUser(String targetUserId);

  /// Mute the user identified by [targetUserId].
  Future<Result<void>> muteUser(String targetUserId);

  /// Unmute the user identified by [targetUserId].
  Future<Result<void>> unmuteUser(String targetUserId);
}
