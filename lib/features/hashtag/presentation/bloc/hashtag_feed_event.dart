part of 'hashtag_feed_bloc.dart';

/// All events that can be dispatched to [HashtagFeedBloc].
sealed class HashtagFeedEvent extends Equatable {
  const HashtagFeedEvent();

  @override
  List<Object?> get props => [];
}

/// Load the first page of posts for the hashtag.
final class HashtagFeedLoadRequested extends HashtagFeedEvent {
  const HashtagFeedLoadRequested();
}

/// Request the next page of the current hashtag feed.
///
/// Silently ignored when the BLoC is in [HashtagFeedTerminated] state.
final class HashtagFeedNextPageRequested extends HashtagFeedEvent {
  const HashtagFeedNextPageRequested();
}
