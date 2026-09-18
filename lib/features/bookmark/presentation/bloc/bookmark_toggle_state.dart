part of 'bookmark_toggle_bloc.dart';

/// All states that [BookmarkToggleBloc] can emit.
sealed class BookmarkToggleState extends Equatable {
  const BookmarkToggleState();

  @override
  List<Object?> get props => [];
}

/// No bookmark action has been requested yet.
final class BookmarkInitial extends BookmarkToggleState {
  const BookmarkInitial();
}

/// A bookmark or unbookmark operation is in progress.
final class BookmarkLoading extends BookmarkToggleState {
  const BookmarkLoading();
}

/// A bookmark or unbookmark operation completed successfully.
final class BookmarkSuccess extends BookmarkToggleState {
  const BookmarkSuccess({required this.isBookmarked});

  /// True when the post is now bookmarked; false when the bookmark was removed.
  final bool isBookmarked;

  @override
  List<Object?> get props => [isBookmarked];
}

/// A bookmark or unbookmark operation failed.
final class BookmarkError extends BookmarkToggleState {
  const BookmarkError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
