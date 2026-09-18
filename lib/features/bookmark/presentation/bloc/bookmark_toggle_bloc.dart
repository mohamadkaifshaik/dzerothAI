// ignore_for_file: prefer_initializing_formals
import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/repositories/bookmark_repository.dart';

part 'bookmark_toggle_event.dart';
part 'bookmark_toggle_state.dart';

/// Manages the bookmark/unbookmark toggle for a single post.
///
/// A new instance is scoped to a specific [postId] to avoid BLoC leakage
/// across cards in a list. It must be provided externally (e.g. at the list
/// level, keyed by postId) rather than created inside PostCard.
class BookmarkToggleBloc
    extends Bloc<BookmarkToggleEvent, BookmarkToggleState> {
  BookmarkToggleBloc({
    required BookmarkRepository bookmarkRepository,
    required String postId,
  }) : _repository = bookmarkRepository,
       _postId = postId,
       super(const BookmarkInitial()) {
    on<BookmarkRequested>(_onBookmarkRequested);
    on<UnbookmarkRequested>(_onUnbookmarkRequested);
  }

  final BookmarkRepository _repository;
  final String _postId;

  Future<void> _onBookmarkRequested(
    BookmarkRequested event,
    Emitter<BookmarkToggleState> emit,
  ) async {
    emit(const BookmarkLoading());
    final result = await _repository.bookmark(_postId);
    switch (result) {
      case Success():
        emit(const BookmarkSuccess(isBookmarked: true));
      case Err(:final failure):
        emit(BookmarkError(failure: failure));
    }
  }

  Future<void> _onUnbookmarkRequested(
    UnbookmarkRequested event,
    Emitter<BookmarkToggleState> emit,
  ) async {
    emit(const BookmarkLoading());
    final result = await _repository.unbookmark(_postId);
    switch (result) {
      case Success():
        emit(const BookmarkSuccess(isBookmarked: false));
      case Err(:final failure):
        emit(BookmarkError(failure: failure));
    }
  }
}
