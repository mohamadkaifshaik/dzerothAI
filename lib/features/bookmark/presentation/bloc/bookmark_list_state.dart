part of 'bookmark_list_bloc.dart';

/// All states that [BookmarkListBloc] can emit.
sealed class BookmarkListState extends Equatable {
  const BookmarkListState();

  @override
  List<Object?> get props => [];
}

/// No bookmarks have been requested yet.
final class BookmarkListInitial extends BookmarkListState {
  const BookmarkListInitial();
}

/// The first page of bookmarks is loading.
final class BookmarkListLoading extends BookmarkListState {
  const BookmarkListLoading();
}

/// At least one page has been loaded and the list is not yet terminated.
///
/// [hasMore] is false while a next-page request is in-flight, preventing
/// duplicate requests.
final class BookmarkListLoaded extends BookmarkListState {
  const BookmarkListLoaded({
    required this.posts,
    required this.nextCursor,
    required this.hasMore,
  });

  final List<Post> posts;
  final String? nextCursor;

  /// True when [nextCursor] is non-null and no next-page request is in-flight.
  final bool hasMore;

  @override
  List<Object?> get props => [posts, nextCursor, hasMore];
}

/// The bookmark list has reached the server-enforced boundary (200 items).
///
/// Per CLAUDE.md §2.1 this is a terminal state. No further pages must be
/// fetched. The UI must show [GoTouchGrassWidget].
final class BookmarkListTerminated extends BookmarkListState {
  const BookmarkListTerminated({required this.posts});

  final List<Post> posts;

  @override
  List<Object?> get props => [posts];
}

/// The bookmark list request failed.
final class BookmarkListError extends BookmarkListState {
  const BookmarkListError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
