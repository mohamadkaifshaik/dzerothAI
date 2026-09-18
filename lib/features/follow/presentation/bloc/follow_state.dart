part of 'follow_bloc.dart';

/// All states that [FollowBloc] can emit.
sealed class FollowState extends Equatable {
  const FollowState();

  @override
  List<Object?> get props => [];
}

/// No follow action has been requested yet.
final class FollowInitial extends FollowState {
  const FollowInitial();
}

/// A follow or unfollow operation is in progress.
final class FollowLoading extends FollowState {
  const FollowLoading();
}

/// A follow or unfollow operation completed successfully.
final class FollowSuccess extends FollowState {
  const FollowSuccess({required this.isFollowing});

  /// True when the caller now follows the target; false when they do not.
  final bool isFollowing;

  @override
  List<Object?> get props => [isFollowing];
}

/// A follow or unfollow operation failed.
final class FollowError extends FollowState {
  const FollowError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
