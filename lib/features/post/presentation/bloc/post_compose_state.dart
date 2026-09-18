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
