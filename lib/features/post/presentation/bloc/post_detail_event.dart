part of 'post_detail_bloc.dart';

/// All events that can be dispatched to [PostDetailBloc].
sealed class PostDetailEvent extends Equatable {
  const PostDetailEvent();

  @override
  List<Object?> get props => [];
}

/// Load a single post by its ID.
final class PostDetailLoadRequested extends PostDetailEvent {
  const PostDetailLoadRequested({required this.postId});

  final String postId;

  @override
  List<Object?> get props => [postId];
}
