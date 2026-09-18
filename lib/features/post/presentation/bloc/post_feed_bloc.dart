import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/entities/post.dart';
import '../../domain/repositories/post_repository.dart';

part 'post_feed_event.dart';
part 'post_feed_state.dart';

/// Discriminates the two kinds of paginated post feed.
enum FeedType {
  /// Posts authored by a specific user (GET /users/{id}/posts).
  authorPosts,

  /// Replies in a thread (GET /posts/{id}/thread).
  threadReplies,
}

/// Manages the state of a finite, cursor-paginated post feed.
///
/// Per CLAUDE.md §2.1 there is NO infinite scrolling.
/// When the repository returns [PostPage.terminated] == true, this BLoC emits
/// [PostFeedTerminated] and ignores all subsequent [PostFeedNextPageRequested]
/// events.
class PostFeedBloc extends Bloc<PostFeedEvent, PostFeedState> {
  PostFeedBloc({required PostRepository postRepository})
    : _repository = postRepository,
      super(const PostFeedInitial()) {
    on<PostFeedLoadRequested>(_onLoadRequested);
    on<PostFeedNextPageRequested>(_onNextPageRequested);
  }

  final PostRepository _repository;

  Future<void> _onLoadRequested(
    PostFeedLoadRequested event,
    Emitter<PostFeedState> emit,
  ) async {
    emit(const PostFeedLoading());

    final result = await _fetch(
      type: event.feedType,
      subjectId: event.subjectId,
      cursor: null,
    );

    switch (result) {
      case Success(:final value):
        if (value.terminated) {
          emit(PostFeedTerminated(posts: value.items));
        } else {
          emit(
            PostFeedLoaded(
              posts: value.items,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
              feedType: event.feedType,
              subjectId: event.subjectId,
            ),
          );
        }
      case Err(:final failure):
        emit(PostFeedError(failure: failure));
    }
  }

  Future<void> _onNextPageRequested(
    PostFeedNextPageRequested event,
    Emitter<PostFeedState> emit,
  ) async {
    // Critical invariant: once terminated, reject all further page requests.
    final current = state;
    if (current is PostFeedTerminated) return;
    if (current is! PostFeedLoaded) return;
    if (!current.hasMore) return;

    // Preserve current posts while loading the next page.
    emit(
      PostFeedLoaded(
        posts: current.posts,
        nextCursor: current.nextCursor,
        hasMore: false, // prevent duplicate requests while loading
        feedType: current.feedType,
        subjectId: current.subjectId,
      ),
    );

    final result = await _fetch(
      type: current.feedType,
      subjectId: current.subjectId,
      cursor: current.nextCursor,
    );

    switch (result) {
      case Success(:final value):
        final merged = [...current.posts, ...value.items];
        if (value.terminated) {
          emit(PostFeedTerminated(posts: merged));
        } else {
          emit(
            PostFeedLoaded(
              posts: merged,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
              feedType: current.feedType,
              subjectId: current.subjectId,
            ),
          );
        }
      case Err(:final failure):
        // Restore the previous loaded state so the user can retry.
        emit(
          PostFeedLoaded(
            posts: current.posts,
            nextCursor: current.nextCursor,
            hasMore: current.nextCursor != null,
            feedType: current.feedType,
            subjectId: current.subjectId,
          ),
        );
        emit(PostFeedError(failure: failure));
    }
  }

  Future<Result<PostPage>> _fetch({
    required FeedType type,
    required String subjectId,
    required String? cursor,
  }) {
    return switch (type) {
      FeedType.authorPosts => _repository.listPostsByAuthor(
        subjectId,
        cursor: cursor,
      ),
      FeedType.threadReplies => _repository.listThreadReplies(
        subjectId,
        cursor: cursor,
      ),
    };
  }
}
