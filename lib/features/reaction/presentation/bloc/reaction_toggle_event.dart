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

/// Seeds the bloc with the server-authoritative reaction state for the viewer.
///
/// Dispatched when the single-post detail endpoint returns [viewer_has_reacted].
/// Does NOT call the API — it only updates local state to match what the server
/// already reported.  Must be idempotent: dispatching it multiple times with
/// the same value must not trigger any API call.
final class ReactionStateHydrated extends ReactionToggleEvent {
  const ReactionStateHydrated({required this.reacted});

  /// True when the authenticated viewer has already reacted to this post.
  final bool reacted;
}
