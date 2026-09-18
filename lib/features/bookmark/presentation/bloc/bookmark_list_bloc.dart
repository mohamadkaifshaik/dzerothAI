import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../../post/domain/entities/post.dart';
import '../../domain/repositories/bookmark_repository.dart';

part 'bookmark_list_event.dart';
part 'bookmark_list_state.dart';

/// Manages the paginated bookmark list for the authenticated user.
///
/// Per CLAUDE.md §2.1 there is NO infinite scrolling.
/// When the repository returns [BookmarkPage.terminated] == true, this BLoC
/// emits [BookmarkListTerminated] and silently ignores all subsequent
/// [BookmarkListNextPageRequested] events.
///
/// The backend terminates at 200 items.
class BookmarkListBloc extends Bloc<BookmarkListEvent, BookmarkListState> {
  BookmarkListBloc({required BookmarkRepository bookmarkRepository})
    : _repository = bookmarkRepository,
      super(const BookmarkListInitial()) {
    on<BookmarkListLoadRequested>(_onLoadRequested);
    on<BookmarkListNextPageRequested>(_onNextPageRequested);
  }

  final BookmarkRepository _repository;

  Future<void> _onLoadRequested(
    BookmarkListLoadRequested event,
    Emitter<BookmarkListState> emit,
  ) async {
    emit(const BookmarkListLoading());

    final result = await _repository.listBookmarks(cursor: null);

    switch (result) {
      case Success(:final value):
        final posts = value.items.map((i) => i.post).toList();
        if (value.terminated) {
          emit(BookmarkListTerminated(posts: posts));
        } else {
          emit(
            BookmarkListLoaded(
              posts: posts,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
            ),
          );
        }
      case Err(:final failure):
        emit(BookmarkListError(failure: failure));
    }
  }

  Future<void> _onNextPageRequested(
    BookmarkListNextPageRequested event,
    Emitter<BookmarkListState> emit,
  ) async {
    // Critical invariant: once terminated, reject all further page requests.
    final current = state;
    if (current is BookmarkListTerminated) return;
    if (current is! BookmarkListLoaded) return;
    if (!current.hasMore) return;

    // Lock hasMore to false while in-flight to prevent duplicate requests.
    emit(
      BookmarkListLoaded(
        posts: current.posts,
        nextCursor: current.nextCursor,
        hasMore: false,
      ),
    );

    final result = await _repository.listBookmarks(cursor: current.nextCursor);

    switch (result) {
      case Success(:final value):
        final newPosts = value.items.map((i) => i.post).toList();
        final merged = [...current.posts, ...newPosts];
        if (value.terminated) {
          emit(BookmarkListTerminated(posts: merged));
        } else {
          emit(
            BookmarkListLoaded(
              posts: merged,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
            ),
          );
        }
      case Err(:final failure):
        // Restore previous loaded state so the user can retry.
        emit(
          BookmarkListLoaded(
            posts: current.posts,
            nextCursor: current.nextCursor,
            hasMore: current.nextCursor != null,
          ),
        );
        emit(BookmarkListError(failure: failure));
    }
  }
}
