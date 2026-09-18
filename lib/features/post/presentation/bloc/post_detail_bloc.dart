import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/entities/post.dart';
import '../../domain/repositories/post_repository.dart';

part 'post_detail_event.dart';
part 'post_detail_state.dart';

/// Manages the state for the single-post detail view.
class PostDetailBloc extends Bloc<PostDetailEvent, PostDetailState> {
  PostDetailBloc({required PostRepository postRepository})
    : _repository = postRepository,
      super(const PostDetailInitial()) {
    on<PostDetailLoadRequested>(_onLoadRequested);
  }

  final PostRepository _repository;

  Future<void> _onLoadRequested(
    PostDetailLoadRequested event,
    Emitter<PostDetailState> emit,
  ) async {
    emit(const PostDetailLoading());

    final result = await _repository.getPost(event.postId);

    switch (result) {
      case Success(:final value):
        emit(PostDetailLoaded(post: value));
      case Err(:final failure):
        emit(PostDetailError(failure: failure));
    }
  }
}
