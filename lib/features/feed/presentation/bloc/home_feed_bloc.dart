import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../../post/domain/entities/post.dart';
import '../../domain/repositories/feed_repository.dart';

part 'home_feed_event.dart';
part 'home_feed_state.dart';

/// Manages the state of the authenticated user's home timeline feed.
///
/// Per CLAUDE.md §2.1 there is NO infinite scrolling.
/// When the repository returns [PostPage.terminated] == true, this BLoC emits
/// [HomeFeedTerminated] and silently ignores all subsequent
/// [HomeFeedNextPageRequested] events.
///
/// The backend determines feed boundaries.  The client must not attempt to
/// bypass or work around the termination signal.
class HomeFeedBloc extends Bloc<HomeFeedEvent, HomeFeedState> {
  HomeFeedBloc({required FeedRepository feedRepository})
    : _repository = feedRepository,
      super(const HomeFeedInitial()) {
    on<HomeFeedLoadRequested>(_onLoadRequested);
    on<HomeFeedNextPageRequested>(_onNextPageRequested);
  }

  final FeedRepository _repository;

  Future<void> _onLoadRequested(
    HomeFeedLoadRequested event,
    Emitter<HomeFeedState> emit,
  ) async {
    emit(const HomeFeedLoading());

    final result = await _repository.getHomeFeed(cursor: null);

    switch (result) {
      case Success(:final value):
        if (value.terminated) {
          if (value.items.isEmpty) {
            // terminated=true with no items: the backend treats an empty
            // follow list as an immediately-terminated feed.
            emit(const HomeFeedEmpty());
          } else {
            emit(HomeFeedTerminated(posts: value.items));
          }
        } else {
          emit(
            HomeFeedLoaded(
              posts: value.items,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
            ),
          );
        }
      case Err(:final failure):
        emit(HomeFeedError(failure: failure));
    }
  }

  Future<void> _onNextPageRequested(
    HomeFeedNextPageRequested event,
    Emitter<HomeFeedState> emit,
  ) async {
    // Critical invariant: once terminated, reject all further page requests.
    final current = state;
    if (current is HomeFeedTerminated) return;
    if (current is! HomeFeedLoaded) return;
    if (!current.hasMore) return;

    // Lock hasMore to false while the request is in-flight to prevent
    // duplicate next-page requests.
    emit(
      HomeFeedLoaded(
        posts: current.posts,
        nextCursor: current.nextCursor,
        hasMore: false,
      ),
    );

    final result = await _repository.getHomeFeed(cursor: current.nextCursor);

    switch (result) {
      case Success(:final value):
        final merged = [...current.posts, ...value.items];
        if (value.terminated) {
          emit(HomeFeedTerminated(posts: merged));
        } else {
          emit(
            HomeFeedLoaded(
              posts: merged,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
            ),
          );
        }
      case Err(:final failure):
        // Restore the previous loaded state so the user can retry.
        emit(
          HomeFeedLoaded(
            posts: current.posts,
            nextCursor: current.nextCursor,
            hasMore: current.nextCursor != null,
          ),
        );
        emit(HomeFeedError(failure: failure));
    }
  }
}
