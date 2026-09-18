part of 'home_feed_bloc.dart';

/// All states that [HomeFeedBloc] can emit.
sealed class HomeFeedState extends Equatable {
  const HomeFeedState();

  @override
  List<Object?> get props => [];
}

/// No feed has been requested yet.
final class HomeFeedInitial extends HomeFeedState {
  const HomeFeedInitial();
}

/// The first page of the feed is loading.
final class HomeFeedLoading extends HomeFeedState {
  const HomeFeedLoading();
}

/// At least one page has been loaded and the feed is not yet terminated.
///
/// [hasMore] is false while a next-page request is in-flight, preventing
/// duplicate requests from being issued.
final class HomeFeedLoaded extends HomeFeedState {
  const HomeFeedLoaded({
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

/// The feed has reached the server-enforced boundary.
///
/// Per CLAUDE.md §2.1 this is a terminal state.  No further pages must be
/// fetched.  The UI must show [GoTouchGrassWidget].
final class HomeFeedTerminated extends HomeFeedState {
  const HomeFeedTerminated({required this.posts});

  final List<Post> posts;

  @override
  List<Object?> get props => [posts];
}

/// The authenticated user follows no one — their timeline is empty.
///
/// Distinct from [HomeFeedTerminated]: this state represents "no content
/// because no follows" rather than "all available content has been shown".
final class HomeFeedEmpty extends HomeFeedState {
  const HomeFeedEmpty();
}

/// The feed request failed.
final class HomeFeedError extends HomeFeedState {
  const HomeFeedError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
