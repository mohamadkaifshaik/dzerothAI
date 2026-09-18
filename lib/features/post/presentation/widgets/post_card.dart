import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:go_router/go_router.dart';

import '../../../../features/auth/presentation/bloc/auth_bloc.dart';
import '../../../../features/bookmark/presentation/bloc/bookmark_toggle_bloc.dart';
import '../../domain/entities/post.dart';

/// Renders a single post in a list or feed.
///
/// IMPORTANT: This widget intentionally renders zero social-validation metrics
/// (likes, impressions, bookmarks, reply counts, repost counts, view counts,
/// share counts, follower counts, or any equivalent popularity metric) per
/// CLAUDE.md §2.3.  Do not add such UI elements to this widget.
///
/// The bookmark toggle icon is shown only when the viewer is authenticated.
/// It dispatches to a [BookmarkToggleBloc] that MUST be provided in the widget
/// tree above this card (keyed by postId at the list level). No count label is
/// ever displayed — only the icon.
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
                  const SizedBox(height: 8),
                  _PostActions(post: post),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Post actions row
// ---------------------------------------------------------------------------

/// Renders the actions row for a post card.
///
/// Currently the only action is the bookmark toggle, visible only when the
/// viewer is authenticated. No count labels are ever shown.
class _PostActions extends StatelessWidget {
  const _PostActions({required this.post});

  final Post post;

  @override
  Widget build(BuildContext context) {
    final authState = context.watch<AuthBloc>().state;
    final isAuthenticated = authState is AuthAuthenticated;

    if (!isAuthenticated) {
      // No actions visible for unauthenticated viewers.
      return const SizedBox.shrink();
    }

    return Row(children: [_BookmarkIcon(post: post)]);
  }
}

/// Bookmark toggle icon button.
///
/// Reads [BookmarkToggleBloc] from context. That BLoC MUST be provided by the
/// list/feed widget above this card, keyed by the post ID, so that state is
/// stable across renders and does not reset on every rebuild.
///
/// PUBLIC METRICS LOCKDOWN: no bookmark count label is ever rendered.
class _BookmarkIcon extends StatelessWidget {
  const _BookmarkIcon({required this.post});

  final Post post;

  @override
  Widget build(BuildContext context) {
    // BookmarkToggleBloc is optional: if none is provided in the tree the
    // bookmark icon is hidden gracefully.
    final bloc = _tryReadBloc(context);
    if (bloc == null) return const SizedBox.shrink();

    return BlocBuilder<BookmarkToggleBloc, BookmarkToggleState>(
      builder: (context, state) {
        final isBookmarked = switch (state) {
          BookmarkSuccess(:final isBookmarked) => isBookmarked,
          _ => false,
        };
        final isLoading = state is BookmarkLoading;

        return Semantics(
          label: isBookmarked ? 'Remove bookmark' : 'Bookmark',
          button: true,
          child: IconButton(
            iconSize: 20,
            visualDensity: VisualDensity.compact,
            padding: EdgeInsets.zero,
            icon: isLoading
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 1.5),
                  )
                : Icon(
                    isBookmarked
                        ? Icons.bookmark
                        : Icons.bookmark_border_outlined,
                  ),
            onPressed: isLoading
                ? null
                : () {
                    if (isBookmarked) {
                      context.read<BookmarkToggleBloc>().add(
                        const UnbookmarkRequested(),
                      );
                    } else {
                      context.read<BookmarkToggleBloc>().add(
                        const BookmarkRequested(),
                      );
                    }
                  },
          ),
        );
      },
    );
  }

  /// Returns the nearest [BookmarkToggleBloc] in the tree, or null if none
  /// has been provided. This avoids a hard crash when PostCard is rendered
  /// in contexts where no bookmark BLoC has been wired up yet.
  BookmarkToggleBloc? _tryReadBloc(BuildContext context) {
    try {
      return context.read<BookmarkToggleBloc>();
    } catch (_) {
      return null;
    }
  }
}

// ---------------------------------------------------------------------------
// Deleted post card
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Avatar
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Author row
// ---------------------------------------------------------------------------

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
