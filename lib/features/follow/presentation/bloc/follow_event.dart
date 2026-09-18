part of 'follow_bloc.dart';

/// All events that can be dispatched to [FollowBloc].
sealed class FollowEvent {
  const FollowEvent();
}

/// Follow the target user.
final class FollowRequested extends FollowEvent {
  const FollowRequested();
}

/// Unfollow the target user.
final class UnfollowRequested extends FollowEvent {
  const UnfollowRequested();
}

/// Check whether the authenticated caller currently follows the target user.
final class FollowStatusCheckRequested extends FollowEvent {
  const FollowStatusCheckRequested();
}
