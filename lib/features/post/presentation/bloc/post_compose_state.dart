part of 'post_compose_bloc.dart';

/// All states that [PostComposeBloc] can emit.
sealed class PostComposeState extends Equatable {
  const PostComposeState();

  @override
  List<Object?> get props => [];
}

/// No submission has been attempted yet.
final class PostComposeInitial extends PostComposeState {
  const PostComposeInitial();
}

/// The mandatory 5-second countdown is running (CLAUDE.md §2.2).
///
/// Applies to 'quote' and 'repost' post types only.  Submission is blocked
/// until [secondsRemaining] reaches 0.  The UI must display [secondsRemaining]
/// and provide a cancel affordance that dispatches
/// [PostComposeCountdownCancelled].
final class PostComposeCountdown extends PostComposeState {
  const PostComposeCountdown({
    required this.secondsRemaining,
    required this.totalSeconds,
  });

  final int secondsRemaining;
  final int totalSeconds;

  @override
  List<Object?> get props => [secondsRemaining, totalSeconds];
}

/// A post creation request is in-flight.
final class PostComposeSubmitting extends PostComposeState {
  const PostComposeSubmitting();
}

/// The post was created successfully.
final class PostComposeSuccess extends PostComposeState {
  const PostComposeSuccess({required this.post});

  final Post post;

  @override
  List<Object?> get props => [post];
}

/// Post creation failed.
final class PostComposeError extends PostComposeState {
  const PostComposeError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
