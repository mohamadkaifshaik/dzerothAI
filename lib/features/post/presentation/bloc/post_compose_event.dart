part of 'post_compose_bloc.dart';

/// All events that can be dispatched to [PostComposeBloc].
sealed class PostComposeEvent extends Equatable {
  const PostComposeEvent();

  @override
  List<Object?> get props => [];
}

/// Submit a new post for creation.
///
/// [postType] must be one of: 'original', 'reply', 'quote', 'repost'.
/// [content] must be provided (and non-empty) for all types except 'repost'.
/// For 'quote' posts [content] must also contain at least 5 distinct words
/// per CLAUDE.md §2.2.
final class PostComposeSubmitted extends PostComposeEvent {
  const PostComposeSubmitted({
    required this.postType,
    this.content,
    this.parentId,
    this.quotedPostId,
  });

  final String postType;
  final String? content;
  final String? parentId;
  final String? quotedPostId;

  @override
  List<Object?> get props => [postType, content, parentId, quotedPostId];
}
