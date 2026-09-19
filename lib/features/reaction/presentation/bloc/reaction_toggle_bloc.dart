// ignore_for_file: prefer_initializing_formals
import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/result.dart';
import '../../domain/repositories/reaction_repository.dart';

part 'reaction_toggle_event.dart';
part 'reaction_toggle_state.dart';

/// Manages the react/unreact toggle for a single post.
///
/// A new instance is scoped to a specific [postId] to avoid BLoC leakage
/// across cards in a list. It must be provided externally (e.g. at the detail
/// screen level, keyed by postId) rather than created inside PostCard.
///
/// [initiallyReacted] comes from [viewer_has_reacted] on the single-post
/// endpoint. It is only available when fetching a post with authentication.
///
/// OPTIMISTIC UPDATE: the UI reflects the toggle immediately; on error the
/// previous state is restored.
///
/// PUBLIC METRICS LOCKDOWN: no reaction count is ever surfaced (CLAUDE.md §2.3).
/// NO 5-SECOND COUNTDOWN: reactions are exempt from share/quote friction
/// (CLAUDE.md §2.2 applies only to share and quote actions).
class ReactionToggleBloc
    extends Bloc<ReactionToggleEvent, ReactionToggleState> {
  ReactionToggleBloc({
    required ReactionRepository reactionRepository,
    required String postId,
    bool initiallyReacted = false,
  }) : _repository = reactionRepository,
       _postId = postId,
       super(
         initiallyReacted
             ? ReactionOn(postId: postId)
             : ReactionOff(postId: postId),
       ) {
    on<ReactionToggleRequested>(_onToggleRequested);
  }

  final ReactionRepository _repository;
  final String _postId;

  Future<void> _onToggleRequested(
    ReactionToggleRequested event,
    Emitter<ReactionToggleState> emit,
  ) async {
    final previousState = state;
    final currentlyReacted = switch (state) {
      ReactionOn() => true,
      _ => false,
    };

    // Optimistic update.
    emit(const ReactionLoading());

    final Result<void> result;
    if (currentlyReacted) {
      result = await _repository.unreact(_postId);
    } else {
      result = await _repository.react(_postId);
    }

    switch (result) {
      case Success():
        if (currentlyReacted) {
          emit(ReactionOff(postId: _postId));
        } else {
          emit(ReactionOn(postId: _postId));
        }
      case Err(:final failure):
        // Revert to previous state on error.
        emit(ReactionError(postId: _postId, message: failure.message));
        emit(previousState);
    }
  }
}
