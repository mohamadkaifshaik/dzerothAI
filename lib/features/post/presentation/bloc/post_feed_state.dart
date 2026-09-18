part of 'post_feed_bloc.dart';

/// All states that [PostFeedBloc] can emit.
sealed class PostFeedState extends Equatable {
  const PostFeedState();

  @override
  List<Object?> get props => [];
}

/// No feed has been requested yet.
final class PostFeedInitial extends PostFeedState {
  const PostFeedInitial();
}

/// The first page of the feed is loading.
final class PostFeedLoading extends PostFeedState {
  const PostFeedLoading();
}

/// At least one page has been loaded and the feed is not yet terminated.
///
/// [hasMore] is false while a next-page request is in-flight.
final class PostFeedLoaded extends PostFeedState {
  const PostFeedLoaded({
    required this.posts,
    required this.nextCursor,
    required this.hasMore,
    this.feedType = FeedType.authorPosts,
    this.subjectId = '',
  });

  final List<Post> posts;
  final String? nextCursor;

  /// True when [nextCursor] is non-null and no next-page request is in-flight.
  final bool hasMore;

  /// Carried forward so [PostFeedBloc._onNextPageRequested] can re-fetch.
  final FeedType feedType;
  final String subjectId;

  @override
  List<Object?> get props => [posts, nextCursor, hasMore, feedType, subjectId];
}

/// The feed has reached the server-enforced boundary.
///
/// Per CLAUDE.md §2.1 this is a terminal state.  No further pages must be
/// fetched, and the UI must show the termination/boundary experience.
final class PostFeedTerminated extends PostFeedState {
  const PostFeedTerminated({required this.posts});

  final List<Post> posts;

  @override
  List<Object?> get props => [posts];
}

/// The feed request failed.
final class PostFeedError extends PostFeedState {
  const PostFeedError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
