part of 'home_feed_bloc.dart';

/// All events that can be dispatched to [HomeFeedBloc].
sealed class HomeFeedEvent extends Equatable {
  const HomeFeedEvent();

  @override
  List<Object?> get props => [];
}

/// Load the first page of the home timeline.
final class HomeFeedLoadRequested extends HomeFeedEvent {
  const HomeFeedLoadRequested();
}

/// Request the next page of the home timeline.
///
/// This event is silently ignored when the BLoC is in [HomeFeedTerminated]
/// state.  The UI must not show a "Load More" affordance when the feed has
/// terminated.  Per CLAUDE.md §2.1 there is no infinite scrolling — the user
/// must explicitly trigger this event (e.g., by pressing a button).
final class HomeFeedNextPageRequested extends HomeFeedEvent {
  const HomeFeedNextPageRequested();
}
