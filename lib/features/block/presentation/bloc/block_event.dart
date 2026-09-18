part of 'block_bloc.dart';

/// All events that can be dispatched to [BlockBloc].
sealed class BlockEvent {
  const BlockEvent();
}

/// Block the target user.
final class BlockUserRequested extends BlockEvent {
  const BlockUserRequested();
}

/// Unblock the target user.
final class UnblockUserRequested extends BlockEvent {
  const UnblockUserRequested();
}

/// Mute the target user.
final class MuteUserRequested extends BlockEvent {
  const MuteUserRequested();
}

/// Unmute the target user.
final class UnmuteUserRequested extends BlockEvent {
  const UnmuteUserRequested();
}
