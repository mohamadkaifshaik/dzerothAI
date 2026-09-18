import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../domain/entities/post.dart';

/// Renders a single post in a list or feed.
///
/// IMPORTANT: This widget intentionally renders zero social-validation metrics
/// (likes, impressions, bookmarks, reply counts, repost counts, view counts,
/// share counts, follower counts, or any equivalent popularity metric) per
/// CLAUDE.md §2.3.  Do not add such UI elements to this widget.
class PostCard extends StatelessWidget {
  const PostCard({super.key, required this.post});

  final Post post;

  @override
  Widget build(BuildContext context) {
    if (post.isDeleted) {
      return _DeletedPostCard(key: key);
    }

    return InkWell(
      onTap: () => context.push('/posts/${post.id}'),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _Avatar(author: post.author),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  _AuthorRow(author: post.author, createdAt: post.createdAt),
                  if (post.content != null && post.content!.isNotEmpty) ...[
                    const SizedBox(height: 4),
                    Text(
                      post.content!,
                      style: Theme.of(context).textTheme.bodyMedium,
                    ),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _DeletedPostCard extends StatelessWidget {
  const _DeletedPostCard({super.key});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: Text(
        'Post deleted.',
        style: Theme.of(context).textTheme.bodyMedium?.copyWith(
          color: Theme.of(context).colorScheme.onSurfaceVariant,
          fontStyle: FontStyle.italic,
        ),
      ),
    );
  }
}

class _Avatar extends StatelessWidget {
  const _Avatar({required this.author});

  final PostAuthor author;

  @override
  Widget build(BuildContext context) {
    final url = author.avatarUrl;
    return GestureDetector(
      onTap: () => context.push('/users/${author.id}'),
      child: CircleAvatar(
        radius: 20,
        backgroundImage: url != null ? NetworkImage(url) : null,
        child: url == null
            ? Text(
                author.displayName.isNotEmpty
                    ? author.displayName[0].toUpperCase()
                    : '?',
                style: const TextStyle(fontWeight: FontWeight.bold),
              )
            : null,
      ),
    );
  }
}

class _AuthorRow extends StatelessWidget {
  const _AuthorRow({required this.author, required this.createdAt});

  final PostAuthor author;
  final DateTime createdAt;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final formatted = _formatTimestamp(createdAt);
    return Row(
      children: [
        Flexible(
          child: Text(
            author.displayName,
            style: theme.textTheme.titleSmall?.copyWith(
              fontWeight: FontWeight.w600,
            ),
            overflow: TextOverflow.ellipsis,
          ),
        ),
        const SizedBox(width: 4),
        Text(
          '@${author.handle}',
          style: theme.textTheme.bodySmall?.copyWith(
            color: theme.colorScheme.onSurfaceVariant,
          ),
          overflow: TextOverflow.ellipsis,
        ),
        const SizedBox(width: 4),
        Text(
          '· $formatted',
          style: theme.textTheme.bodySmall?.copyWith(
            color: theme.colorScheme.onSurfaceVariant,
          ),
        ),
      ],
    );
  }

  String _formatTimestamp(DateTime dt) {
    final now = DateTime.now();
    final diff = now.difference(dt);
    if (diff.inSeconds < 60) return '${diff.inSeconds}s';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m';
    if (diff.inHours < 24) return '${diff.inHours}h';
    final months = const [
      'Jan',
      'Feb',
      'Mar',
      'Apr',
      'May',
      'Jun',
      'Jul',
      'Aug',
      'Sep',
      'Oct',
      'Nov',
      'Dec',
    ];
    return '${months[dt.month - 1]} ${dt.day}';
  }
}
