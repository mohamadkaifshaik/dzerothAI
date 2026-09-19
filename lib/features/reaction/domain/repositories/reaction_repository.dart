import '../../../../core/error/result.dart';

/// Contract for reaction (heart) toggle operations on posts.
///
/// PUBLIC METRICS LOCKDOWN: Reaction counts are never exposed per
/// CLAUDE.md §2.3. The toggle UI shows an icon only — no count label.
abstract class ReactionRepository {
  /// React to the post identified by [postId].
  Future<Result<void>> react(String postId);

  /// Remove the reaction for [postId].
  Future<Result<void>> unreact(String postId);
}
