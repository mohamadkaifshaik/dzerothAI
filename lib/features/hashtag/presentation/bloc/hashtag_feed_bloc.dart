import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../../post/domain/entities/post.dart';
import '../../domain/repositories/hashtag_repository.dart';

part 'hashtag_feed_event.dart';
part 'hashtag_feed_state.dart';

/// Manages the state of a finite, cursor-paginated hashtag feed.
///
/// Per CLAUDE.md §2.1 there is NO infinite scrolling.
/// When the repository returns [PostPage.terminated] == true, this BLoC emits
/// [HashtagFeedTerminated] and ignores all subsequent
/// [HashtagFeedNextPageRequested] events.
class HashtagFeedBloc extends Bloc<HashtagFeedEvent, HashtagFeedState> {
  HashtagFeedBloc({
    required HashtagRepository hashtagRepository,
    required this._tag,
  }) : _repository = hashtagRepository,
       super(const HashtagFeedInitial()) {
    on<HashtagFeedLoadRequested>(_onLoadRequested);
    on<HashtagFeedNextPageRequested>(_onNextPageRequested);
  }

  final HashtagRepository _repository;
  final String _tag;

  Future<void> _onLoadRequested(
    HashtagFeedLoadRequested event,
    Emitter<HashtagFeedState> emit,
  ) async {
    emit(const HashtagFeedLoading());

    final result = await _repository.getHashtagFeed(tag: _tag, cursor: null);

    switch (result) {
      case Success(:final value):
        if (value.items.isEmpty) {
          emit(const HashtagFeedEmpty());
        } else if (value.terminated) {
          emit(HashtagFeedTerminated(posts: value.items));
        } else {
          emit(
            HashtagFeedLoaded(
              posts: value.items,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
            ),
          );
        }
      case Err(:final failure):
        emit(HashtagFeedError(failure: failure));
    }
  }

  Future<void> _onNextPageRequested(
    HashtagFeedNextPageRequested event,
    Emitter<HashtagFeedState> emit,
  ) async {
    // Critical invariant: once terminated, reject all further page requests.
    final current = state;
    if (current is HashtagFeedTerminated) return;
    if (current is! HashtagFeedLoaded) return;
    if (!current.hasMore) return;

    // Preserve current posts while loading the next page.
    emit(
      HashtagFeedLoaded(
        posts: current.posts,
        nextCursor: current.nextCursor,
        hasMore: false, // prevent duplicate requests while loading
      ),
    );

    final result = await _repository.getHashtagFeed(
      tag: _tag,
      cursor: current.nextCursor,
    );

    switch (result) {
      case Success(:final value):
        final merged = [...current.posts, ...value.items];
        if (value.terminated || value.items.isEmpty) {
          emit(HashtagFeedTerminated(posts: merged));
        } else {
          emit(
            HashtagFeedLoaded(
              posts: merged,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
            ),
          );
        }
      case Err(:final failure):
        // Restore the previous loaded state so the user can retry.
        emit(
          HashtagFeedLoaded(
            posts: current.posts,
            nextCursor: current.nextCursor,
            hasMore: current.nextCursor != null,
          ),
        );
        emit(HashtagFeedError(failure: failure));
    }
  }
}
