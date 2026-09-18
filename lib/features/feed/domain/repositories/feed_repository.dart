import '../../../../core/error/result.dart';
import '../../../post/domain/entities/post.dart';

/// Contract for home-feed data operations.
///
/// [PostPage] (defined in the post domain) is reused here — it already
/// carries [PostPage.terminated] which this repository's callers must
/// respect per CLAUDE.md §2.1.
abstract class FeedRepository {
  /// Fetches a page of posts for the authenticated user's home timeline.
  ///
  /// Pass [cursor] from the previous [PostPage.nextCursor] to load the next
  /// page.  When [PostPage.terminated] is true the BLoC must enter
  /// [HomeFeedTerminated] and never fetch again.
  Future<Result<PostPage>> getHomeFeed({String? cursor});
}
