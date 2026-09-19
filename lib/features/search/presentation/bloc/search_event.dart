part of 'search_bloc.dart';

/// All events that can be dispatched to [SearchBloc].
sealed class SearchEvent {
  const SearchEvent();
}

/// Dispatched whenever the user changes the search input text.
///
/// The BLoC debounces this event by 300 ms before firing a network request.
/// An empty [query] causes [SearchInitial] to be emitted without a request.
final class SearchQueryChanged extends SearchEvent {
  const SearchQueryChanged({required this.query});

  final String query;
}

/// Internal event emitted after the 300 ms debounce timer fires.
///
/// Not intended to be dispatched externally. Use [SearchQueryChanged].
final class _SearchQueryDebounced extends SearchEvent {
  const _SearchQueryDebounced({required this.query});

  final String query;
}

/// Request the next page of post results for the current query.
///
/// Silently ignored when the BLoC is in [SearchPostsTerminated] state or
/// when [SearchPostsLoaded.hasMore] is false.
final class SearchPostsNextPageRequested extends SearchEvent {
  const SearchPostsNextPageRequested();
}

/// Request the next page of user results for the current query.
///
/// Silently ignored when the BLoC is in [SearchUsersTerminated] state or
/// when [SearchUsersLoaded.hasMore] is false.
final class SearchUsersNextPageRequested extends SearchEvent {
  const SearchUsersNextPageRequested();
}

/// Explicitly request user search results for [query].
///
/// Dispatched by the screen when the Users tab becomes active and user results
/// have not yet been fetched for the current query.
final class SearchUsersRequested extends SearchEvent {
  const SearchUsersRequested({required this.query});

  final String query;
}
