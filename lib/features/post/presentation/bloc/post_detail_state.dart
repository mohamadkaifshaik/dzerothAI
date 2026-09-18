part of 'post_detail_bloc.dart';

/// All states that [PostDetailBloc] can emit.
sealed class PostDetailState extends Equatable {
  const PostDetailState();

  @override
  List<Object?> get props => [];
}

/// No post has been requested yet.
final class PostDetailInitial extends PostDetailState {
  const PostDetailInitial();
}

/// A post load is in progress.
final class PostDetailLoading extends PostDetailState {
  const PostDetailLoading();
}

/// A post was loaded successfully.
final class PostDetailLoaded extends PostDetailState {
  const PostDetailLoaded({required this.post});

  final Post post;

  @override
  List<Object?> get props => [post];
}

/// Loading the post failed.
final class PostDetailError extends PostDetailState {
  const PostDetailError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
