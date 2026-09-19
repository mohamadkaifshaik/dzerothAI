part of 'reaction_toggle_bloc.dart';

/// All events that can be dispatched to [ReactionToggleBloc].
sealed class ReactionToggleEvent {
  const ReactionToggleEvent();
}

/// Toggle the reaction state for the target post.
///
/// If currently reacted this removes the reaction; if currently unreacted
/// this adds one.
final class ReactionToggleRequested extends ReactionToggleEvent {
  const ReactionToggleRequested();
}
