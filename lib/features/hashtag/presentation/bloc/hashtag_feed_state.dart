part of 'hashtag_feed_bloc.dart';

/// All states that [HashtagFeedBloc] can emit.
sealed class HashtagFeedState extends Equatable {
  const HashtagFeedState();

  @override
  List<Object?> get props => [];
}

/// No feed has been requested yet.
final class HashtagFeedInitial extends HashtagFeedState {
  const HashtagFeedInitial();
}

/// The first page of posts is loading.
final class HashtagFeedLoading extends HashtagFeedState {
  const HashtagFeedLoading();
}

/// At least one page has been loaded and the feed is not yet terminated.
///
/// [hasMore] is false while a next-page request is in-flight.
final class HashtagFeedLoaded extends HashtagFeedState {
  const HashtagFeedLoaded({
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

/// The feed has reached the server-enforced boundary (200 items).
///
/// Per CLAUDE.md §2.1 this is a terminal state. No further pages must be
/// fetched and the UI must show the GoTouchGrassWidget.
final class HashtagFeedTerminated extends HashtagFeedState {
  const HashtagFeedTerminated({required this.posts});

  final List<Post> posts;

  @override
  List<Object?> get props => [posts];
}

/// No posts exist for this hashtag.
final class HashtagFeedEmpty extends HashtagFeedState {
  const HashtagFeedEmpty();
}

/// The feed request failed.
final class HashtagFeedError extends HashtagFeedState {
  const HashtagFeedError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
