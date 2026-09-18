import 'dart:async';

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
///
/// Per CLAUDE.md §2.2, 'quote' and 'repost' post types require a mandatory
/// 5-second countdown before submission.  The countdown state machine is:
///
///   PostComposeSubmitted (quote/repost)
///     → validation passes
///     → PostComposeCountdown(5)
///     → PostComposeCountdown(4)
///     → ... (timer ticks via PostComposeCountdownTicked)
///     → PostComposeCountdown(1)
///     → PostComposeSubmitting  (when remaining reaches 0)
///     → PostComposeSuccess / PostComposeError
///
/// The countdown can be cancelled at any point before reaching 0 by
/// dispatching [PostComposeCountdownCancelled], which resets to
/// [PostComposeInitial].
///
/// 'original' and 'reply' post types bypass the countdown entirely.
class PostComposeBloc extends Bloc<PostComposeEvent, PostComposeState> {
  PostComposeBloc({required PostRepository postRepository})
    : _repository = postRepository,
      super(const PostComposeInitial()) {
    on<PostComposeSubmitted>(_onSubmitted);
    on<PostComposeCountdownTicked>(_onCountdownTicked);
    on<PostComposeCountdownCancelled>(_onCountdownCancelled);
    on<_PostComposeDoSubmit>(_onDoSubmit);
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

  /// The total seconds of the mandatory share/quote countdown (CLAUDE.md §2.2).
  static const int countdownSeconds = 5;

  // Holds the pending submission parameters during the countdown phase so
  // that _submitPost can use them when the countdown reaches zero.
  Timer? _countdownTimer;
  PostComposeSubmitted? _pendingSubmission;

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

    // --- Countdown gate (CLAUDE.md §2.2) ---
    // Quote and repost types require the mandatory 5-second countdown.
    if (event.postType == 'quote' || event.postType == 'repost') {
      _startCountdown(event, emit);
      return;
    }

    // --- Direct submission for 'original' and 'reply' ---
    await _submitPost(event, emit);
  }

  /// Starts (or restarts) the 5-second countdown timer.
  void _startCountdown(
    PostComposeSubmitted event,
    Emitter<PostComposeState> emit,
  ) {
    _countdownTimer?.cancel();
    _pendingSubmission = event;

    int remaining = countdownSeconds;
    emit(
      PostComposeCountdown(
        secondsRemaining: remaining,
        totalSeconds: countdownSeconds,
      ),
    );

    _countdownTimer = Timer.periodic(const Duration(seconds: 1), (timer) {
      remaining--;
      add(PostComposeCountdownTicked(remaining));
    });
  }

  void _onCountdownTicked(
    PostComposeCountdownTicked event,
    Emitter<PostComposeState> emit,
  ) {
    // Guard: ignore stale ticks if we are no longer in countdown state.
    if (state is! PostComposeCountdown) {
      _countdownTimer?.cancel();
      return;
    }

    if (event.remaining <= 0) {
      _countdownTimer?.cancel();
      _countdownTimer = null;
      // Proceed with the actual submission using the pending event.
      final pending = _pendingSubmission;
      _pendingSubmission = null;
      if (pending != null) {
        // Use add() to re-enter the event loop so that the async submission
        // is handled correctly by the BLoC.
        add(_PostComposeDoSubmit(pending));
      }
    } else {
      emit(
        PostComposeCountdown(
          secondsRemaining: event.remaining,
          totalSeconds: countdownSeconds,
        ),
      );
    }
  }

  void _onCountdownCancelled(
    PostComposeCountdownCancelled event,
    Emitter<PostComposeState> emit,
  ) {
    _countdownTimer?.cancel();
    _countdownTimer = null;
    _pendingSubmission = null;
    emit(const PostComposeInitial());
  }

  Future<void> _onDoSubmit(
    _PostComposeDoSubmit event,
    Emitter<PostComposeState> emit,
  ) async {
    await _submitPost(event.submitted, emit);
  }

  Future<void> _submitPost(
    PostComposeSubmitted event,
    Emitter<PostComposeState> emit,
  ) async {
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

  @override
  Future<void> close() {
    _countdownTimer?.cancel();
    _countdownTimer = null;
    return super.close();
  }
}

// ---------------------------------------------------------------------------
// Internal event — not part of the public API
// ---------------------------------------------------------------------------

/// Internal event that triggers the actual HTTP submission after the countdown
/// reaches zero.  Clients must never dispatch this directly.
final class _PostComposeDoSubmit extends PostComposeEvent {
  const _PostComposeDoSubmit(this.submitted);

  final PostComposeSubmitted submitted;

  @override
  List<Object?> get props => [submitted];
}
