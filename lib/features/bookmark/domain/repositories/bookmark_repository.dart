import '../../../../core/error/result.dart';
import '../entities/bookmark_page.dart';

/// Contract for bookmark toggle and bookmark-list operations.
///
/// PUBLIC METRICS LOCKDOWN: Bookmark counts are never exposed per
/// CLAUDE.md §2.3. The toggle UI shows an icon only — no count label.
abstract class BookmarkRepository {
  /// Bookmark the post identified by [postId].
  Future<Result<void>> bookmark(String postId);

  /// Remove the bookmark for [postId].
  Future<Result<void>> unbookmark(String postId);

  /// List the authenticated user's bookmarks, paginated by [cursor].
  Future<Result<BookmarkPage>> listBookmarks({String? cursor});
}
