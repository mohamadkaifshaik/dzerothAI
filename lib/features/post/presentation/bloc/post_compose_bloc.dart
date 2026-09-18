import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/entities/post.dart';
import '../../domain/repositories/post_repository.dart';

part 'post_compose_event.dart';
part 'post_compose_state.dart';

/// Manages the post-composition workflow.
///
/// Client-side validation is applied as a UX guard before submission.
/// The backend is authoritative for all invariants.
class PostComposeBloc extends Bloc<PostComposeEvent, PostComposeState> {
  PostComposeBloc({required PostRepository postRepository})
    : _repository = postRepository,
      super(const PostComposeInitial()) {
    on<PostComposeSubmitted>(_onSubmitted);
  }

  final PostRepository _repository;

  /// Maximum Unicode code-point length for post content (must match backend).
  static const int maxContentRunes = 500;

  /// Minimum distinct-word count for quote content (CLAUDE.md §2.2).
  ///
  /// Must match backend normalization:
  ///   split on whitespace → lowercase → strip leading/trailing non-letter/
  ///   non-digit characters → count unique tokens.
  static const int minQuoteDistinctWords = 5;

  Future<void> _onSubmitted(
    PostComposeSubmitted event,
    Emitter<PostComposeState> emit,
  ) async {
    // --- Client-side validation (UX guard; backend is authoritative) ---

    if (event.postType != 'repost') {
      final content = event.content ?? '';
      if (content.isEmpty) {
        emit(
          const PostComposeError(
            failure: ValidationFailure(
              message: 'Post content cannot be empty.',
            ),
          ),
        );
        return;
      }

      if (content.runes.length > maxContentRunes) {
        emit(
          PostComposeError(
            failure: ValidationFailure(
              message:
                  'Post content exceeds $maxContentRunes Unicode code points.',
            ),
          ),
        );
        return;
      }

      if (event.postType == 'quote') {
        final distinctCount = _countDistinctWords(content);
        if (distinctCount < minQuoteDistinctWords) {
          emit(
            PostComposeError(
              failure: ValidationFailure(
                message:
                    'Quote posts require at least $minQuoteDistinctWords distinct words.',
              ),
            ),
          );
          return;
        }
      }
    }

    emit(const PostComposeSubmitting());

    final result = await _repository.createPost(
      postType: event.postType,
      content: event.content,
      parentId: event.parentId,
      quotedPostId: event.quotedPostId,
    );

    switch (result) {
      case Success(:final value):
        emit(PostComposeSuccess(post: value));
      case Err(:final failure):
        emit(PostComposeError(failure: failure));
    }
  }

  /// Counts distinct normalized words in [content].
  ///
  /// Normalization steps (must match backend service implementation):
  ///   1. Split on whitespace.
  ///   2. Lowercase each token.
  ///   3. Strip leading and trailing characters that are neither a letter
  ///      nor a digit (Unicode-aware via [RegExp]).
  ///   4. Discard empty tokens.
  ///   5. Count unique tokens.
  ///
  /// This method must remain consistent with the Go backend word-count logic
  /// in the post service so that client and server agree on validity.
  static int _countDistinctWords(String content) {
    final strip = RegExp(r'^[^\p{L}\p{N}]+|[^\p{L}\p{N}]+$', unicode: true);
    final tokens = content.split(RegExp(r'\s+'));
    final distinct = <String>{};
    for (final token in tokens) {
      final normalized = token.toLowerCase().replaceAll(strip, '');
      if (normalized.isNotEmpty) {
        distinct.add(normalized);
      }
    }
    return distinct.length;
  }
}
