part of 'reaction_toggle_bloc.dart';

/// All states that [ReactionToggleBloc] can emit.
sealed class ReactionToggleState extends Equatable {
  const ReactionToggleState();

  @override
  List<Object?> get props => [];
}

/// The viewer has reacted to this post.
final class ReactionOn extends ReactionToggleState {
  const ReactionOn({required this.postId});

  final String postId;

  @override
  List<Object?> get props => [postId];
}

/// The viewer has not reacted to this post (or has removed their reaction).
final class ReactionOff extends ReactionToggleState {
  const ReactionOff({required this.postId});

  final String postId;

  @override
  List<Object?> get props => [postId];
}

/// A react or unreact operation is in progress.
final class ReactionLoading extends ReactionToggleState {
  const ReactionLoading();
}

/// A react or unreact operation failed.
///
/// The bloc automatically reverts to the prior state after emitting this.
final class ReactionError extends ReactionToggleState {
  const ReactionError({required this.postId, required this.message});

  final String postId;
  final String message;

  @override
  List<Object?> get props => [postId, message];
}
