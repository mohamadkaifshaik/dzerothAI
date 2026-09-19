import '../../../../core/error/result.dart';
import '../../../post/domain/entities/post.dart';
import '../entities/user_search_result.dart';

/// Page of post search results.
///
/// [terminated] is true when the server-enforced boundary (50 items) has been
/// reached. Per CLAUDE.md §2.1 the client must stop fetching and display the
/// termination experience when this is true.
class SearchPostPage {
  const SearchPostPage({
    required this.items,
    this.nextCursor,
    required this.terminated,
  });

  final List<Post> items;

  /// Opaque cursor for the next page. Null when there are no further pages.
  final String? nextCursor;

  /// True when the server-enforced search boundary has been reached.
  final bool terminated;
}

/// Page of user search results.
///
/// [terminated] is true when the server-enforced boundary (50 items) has been
/// reached. Per CLAUDE.md §2.1 the client must stop fetching and display the
/// termination experience when this is true.
class SearchUserPage {
  const SearchUserPage({
    required this.items,
    this.nextCursor,
    required this.terminated,
  });

  final List<UserSearchResult> items;

  /// Opaque cursor for the next page. Null when there are no further pages.
  final String? nextCursor;

  /// True when the server-enforced search boundary has been reached.
  final bool terminated;
}

/// Contract for search data operations.
///
/// Both endpoints are auth-optional — unauthenticated callers receive the same
/// results, the Dio interceptor attaches a token only when one is available.
abstract class SearchRepository {
  /// Fetches a page of post search results for [query].
  ///
  /// Pass [cursor] from the previous [SearchPostPage.nextCursor] to load the
  /// next page. When [SearchPostPage.terminated] is true the BLoC must enter
  /// [SearchPostsTerminated] and never fetch again.
  Future<Result<SearchPostPage>> searchPosts({
    required String query,
    String? cursor,
  });

  /// Fetches a page of user search results for [query].
  ///
  /// Pass [cursor] from the previous [SearchUserPage.nextCursor] to load the
  /// next page. When [SearchUserPage.terminated] is true the BLoC must enter
  /// [SearchUsersTerminated] and never fetch again.
  Future<Result<SearchUserPage>> searchUsers({
    required String query,
    String? cursor,
  });
}
