import '../../../../core/error/result.dart';

/// Contract for post and user report operations.
///
/// Reporter identity is never exposed in the UI — these operations are
/// fire-and-forget from the client's perspective.  No countdown is required
/// for reports (CLAUDE.md §2.2 covers share/quote only).
abstract class ReportRepository {
  /// Report the post identified by [postId].
  ///
  /// [reason] must be one of the recognised reason codes.
  /// [detail] is optional free-text (max 500 chars — backend is authoritative).
  Future<Result<void>> reportPost({
    required String postId,
    required String reason,
    String? detail,
  });

  /// Report the user identified by [userId].
  ///
  /// [reason] must be one of the recognised reason codes.
  /// [detail] is optional free-text (max 500 chars — backend is authoritative).
  Future<Result<void>> reportUser({
    required String userId,
    required String reason,
    String? detail,
  });
}
