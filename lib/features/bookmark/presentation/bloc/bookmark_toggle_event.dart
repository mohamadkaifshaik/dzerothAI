part of 'bookmark_toggle_bloc.dart';

/// All events that can be dispatched to [BookmarkToggleBloc].
sealed class BookmarkToggleEvent {
  const BookmarkToggleEvent();
}

/// Bookmark the target post.
final class BookmarkRequested extends BookmarkToggleEvent {
  const BookmarkRequested();
}

/// Remove the bookmark from the target post.
final class UnbookmarkRequested extends BookmarkToggleEvent {
  const UnbookmarkRequested();
}
