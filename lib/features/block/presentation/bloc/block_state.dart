part of 'block_bloc.dart';

/// All states that [BlockBloc] can emit.
sealed class BlockState extends Equatable {
  const BlockState();

  @override
  List<Object?> get props => [];
}

/// No block or mute action has been requested yet.
final class BlockInitial extends BlockState {
  const BlockInitial();
}

/// A block, unblock, mute, or unmute operation is in progress.
final class BlockLoading extends BlockState {
  const BlockLoading();
}

/// A block, unblock, mute, or unmute operation completed successfully.
///
/// [action] is one of: "blocked", "unblocked", "muted", "unmuted".
final class BlockSuccess extends BlockState {
  const BlockSuccess({required this.action});

  final String action;

  @override
  List<Object?> get props => [action];
}

/// A block, unblock, mute, or unmute operation failed.
final class BlockError extends BlockState {
  const BlockError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
