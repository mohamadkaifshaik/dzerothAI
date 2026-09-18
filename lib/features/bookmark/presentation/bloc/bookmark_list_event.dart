part of 'bookmark_list_bloc.dart';

/// All events that can be dispatched to [BookmarkListBloc].
sealed class BookmarkListEvent {
  const BookmarkListEvent();
}

/// Load the first page of bookmarks.
final class BookmarkListLoadRequested extends BookmarkListEvent {
  const BookmarkListLoadRequested();
}

/// Load the next page of bookmarks.
///
/// Ignored when the BLoC is in [BookmarkListTerminated] state.
final class BookmarkListNextPageRequested extends BookmarkListEvent {
  const BookmarkListNextPageRequested();
}
