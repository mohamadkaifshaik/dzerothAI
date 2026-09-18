import 'package:equatable/equatable.dart';

import '../../../post/domain/entities/post.dart';

/// A single bookmark entry belonging to the authenticated user.
class BookmarkItem extends Equatable {
  const BookmarkItem({
    required this.postId,
    required this.createdAt,
    required this.post,
  });

  final String postId;
  final DateTime createdAt;
  final Post post;

  @override
  List<Object?> get props => [postId, createdAt, post];
}
