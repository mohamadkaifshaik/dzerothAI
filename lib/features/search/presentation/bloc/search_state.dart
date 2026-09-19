part of 'search_bloc.dart';

/// All states that [SearchBloc] can emit.
sealed class SearchState extends Equatable {
  const SearchState();

  @override
  List<Object?> get props => [];
}

/// No search has been requested yet, or the query was cleared.
final class SearchInitial extends SearchState {
  const SearchInitial();
}

/// A search request is in-flight for [query].
final class SearchLoading extends SearchState {
  const SearchLoading({required this.query});

  final String query;

  @override
  List<Object?> get props => [query];
}

/// Post results have been loaded for [query] and the list is not yet
/// terminated.
///
/// [hasMore] is false while a next-page request is in-flight, preventing
/// duplicate requests.
final class SearchPostsLoaded extends SearchState {
  const SearchPostsLoaded({
    required this.posts,
    required this.nextCursor,
    required this.hasMore,
    required this.query,
  });

  final List<Post> posts;
  final String? nextCursor;

  /// True when [nextCursor] is non-null and no next-page request is in-flight.
  final bool hasMore;
  final String query;

  @override
  List<Object?> get props => [posts, nextCursor, hasMore, query];
}

/// Post search has reached the server-enforced boundary (50 items).
///
/// Per CLAUDE.md §2.1 this is a terminal state. No further pages must be
/// fetched. The UI must show [GoTouchGrassWidget].
final class SearchPostsTerminated extends SearchState {
  const SearchPostsTerminated({required this.posts, required this.query});

  final List<Post> posts;
  final String query;

  @override
  List<Object?> get props => [posts, query];
}

/// User results have been loaded for [query] and the list is not yet
/// terminated.
///
/// [hasMore] is false while a next-page request is in-flight, preventing
/// duplicate requests.
final class SearchUsersLoaded extends SearchState {
  const SearchUsersLoaded({
    required this.users,
    required this.nextCursor,
    required this.hasMore,
    required this.query,
  });

  final List<UserSearchResult> users;
  final String? nextCursor;

  /// True when [nextCursor] is non-null and no next-page request is in-flight.
  final bool hasMore;
  final String query;

  @override
  List<Object?> get props => [users, nextCursor, hasMore, query];
}

/// User search has reached the server-enforced boundary (50 items).
///
/// Per CLAUDE.md §2.1 this is a terminal state. No further pages must be
/// fetched. The UI must show [GoTouchGrassWidget].
final class SearchUsersTerminated extends SearchState {
  const SearchUsersTerminated({required this.users, required this.query});

  final List<UserSearchResult> users;
  final String query;

  @override
  List<Object?> get props => [users, query];
}

/// The search returned no results for [query].
///
/// [searchType] indicates whether the empty result came from a post search
/// ('posts') or a user search ('users'), allowing each tab to render
/// the appropriate empty-state message.
final class SearchEmpty extends SearchState {
  const SearchEmpty({required this.query, this.searchType = 'posts'});

  final String query;

  /// Either 'posts' or 'users'.
  final String searchType;

  @override
  List<Object?> get props => [query, searchType];
}

/// A search request failed.
final class SearchError extends SearchState {
  const SearchError({required this.message, this.query = ''});

  final String message;
  final String query;

  @override
  List<Object?> get props => [message, query];
}
