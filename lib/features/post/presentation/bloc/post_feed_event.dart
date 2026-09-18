part of 'post_feed_bloc.dart';

/// All events that can be dispatched to [PostFeedBloc].
sealed class PostFeedEvent extends Equatable {
  const PostFeedEvent();

  @override
  List<Object?> get props => [];
}

/// Load the first page of the feed.
///
/// [subjectId] is either a user ID (for [FeedType.authorPosts]) or a thread
/// root post ID (for [FeedType.threadReplies]).
final class PostFeedLoadRequested extends PostFeedEvent {
  const PostFeedLoadRequested({
    required this.subjectId,
    required this.feedType,
  });

  final String subjectId;
  final FeedType feedType;

  @override
  List<Object?> get props => [subjectId, feedType];
}

/// Request the next page of the current feed.
///
/// This event is silently ignored when the BLoC is in [PostFeedTerminated]
/// state.  The UI must not show a "load more" affordance when terminated.
final class PostFeedNextPageRequested extends PostFeedEvent {
  const PostFeedNextPageRequested();
}
