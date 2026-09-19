import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:go_router/go_router.dart';

import '../../../reaction/presentation/bloc/reaction_toggle_bloc.dart';
import '../bloc/post_detail_bloc.dart';
import '../bloc/post_feed_bloc.dart';
import '../widgets/post_thread_view.dart';

/// Displays a single post and its thread of replies.
///
/// No social-validation metrics (likes, impressions, bookmarks, etc.) are
/// rendered anywhere on this screen per CLAUDE.md §2.3.
class PostDetailScreen extends StatelessWidget {
  const PostDetailScreen({super.key, required this.postId});

  final String postId;

  @override
  Widget build(BuildContext context) {
    return MultiBlocListener(
      listeners: [
        BlocListener<PostDetailBloc, PostDetailState>(
          listener: (context, state) {
            // When the post loads successfully, seed ReactionToggleBloc with
            // the server-authoritative viewer reaction state so the toggle
            // reflects whether the authenticated user has already reacted.
            if (state is PostDetailLoaded) {
              final viewerHasReacted = state.post.viewerHasReacted;
              if (viewerHasReacted != null) {
                context.read<ReactionToggleBloc>().add(
                  ReactionStateHydrated(reacted: viewerHasReacted),
                );
              }
            }
          },
        ),
      ],
      child: Scaffold(
        appBar: AppBar(
          leading: BackButton(onPressed: () => context.pop()),
          title: const Text('Post'),
        ),
        body: BlocBuilder<PostDetailBloc, PostDetailState>(
          builder: (context, state) {
            return switch (state) {
              PostDetailInitial() => const SizedBox.shrink(),
              PostDetailLoading() => const Center(
                child: CircularProgressIndicator(),
              ),
              PostDetailLoaded(:final post) => _PostDetailContent(
                post: post,
                postId: postId,
              ),
              PostDetailError(:final failure) => _ErrorView(
                message: failure.message,
                onRetry: () => context.read<PostDetailBloc>().add(
                  PostDetailLoadRequested(postId: postId),
                ),
              ),
            };
          },
        ),
      ),
    );
  }
}

class _PostDetailContent extends StatelessWidget {
  const _PostDetailContent({required this.post, required this.postId});

  final dynamic post;
  final String postId;

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Post detail header.
          if (post.isDeleted)
            const Padding(
              padding: EdgeInsets.all(16),
              child: Text(
                'This post has been deleted.',
                style: TextStyle(fontStyle: FontStyle.italic),
              ),
            )
          else
            _PostHeader(post: post),
          const Divider(height: 1),
          // Thread replies via PostThreadView.
          PostThreadView(
            onRetry: () => context.read<PostFeedBloc>().add(
              PostFeedLoadRequested(
                subjectId: postId,
                feedType: FeedType.threadReplies,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _PostHeader extends StatelessWidget {
  const _PostHeader({required this.post});

  final dynamic post;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final author = post.author;
    final avatarUrl = author.avatarUrl as String?;

    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              GestureDetector(
                onTap: () => context.push('/users/${author.id}'),
                child: CircleAvatar(
                  radius: 22,
                  backgroundImage: avatarUrl != null
                      ? NetworkImage(avatarUrl)
                      : null,
                  child: avatarUrl == null
                      ? Text(
                          (author.displayName as String).isNotEmpty
                              ? (author.displayName as String)[0].toUpperCase()
                              : '?',
                          style: const TextStyle(fontWeight: FontWeight.bold),
                        )
                      : null,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      author.displayName as String,
                      style: theme.textTheme.titleSmall?.copyWith(
                        fontWeight: FontWeight.w600,
                      ),
                      overflow: TextOverflow.ellipsis,
                    ),
                    Text(
                      '@${author.handle}',
                      style: theme.textTheme.bodySmall?.copyWith(
                        color: theme.colorScheme.onSurfaceVariant,
                      ),
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
                ),
              ),
            ],
          ),
          if (post.content != null && (post.content as String).isNotEmpty) ...[
            const SizedBox(height: 12),
            Text(post.content as String, style: theme.textTheme.bodyLarge),
          ],
          const SizedBox(height: 12),
          Text(
            _formatFullTimestamp(post.createdAt as DateTime),
            style: theme.textTheme.bodySmall?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
            ),
          ),
        ],
      ),
    );
  }

  String _formatFullTimestamp(DateTime dt) {
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
    final hour = dt.hour % 12 == 0 ? 12 : dt.hour % 12;
    final amPm = dt.hour < 12 ? 'AM' : 'PM';
    final min = dt.minute.toString().padLeft(2, '0');
    return '$hour:$min $amPm · ${months[dt.month - 1]} ${dt.day}, ${dt.year}';
  }
}

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.message, required this.onRetry});

  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.error_outline,
              size: 48,
              color: Theme.of(context).colorScheme.error,
            ),
            const SizedBox(height: 12),
            Text(
              message,
              style: Theme.of(context).textTheme.bodyMedium
                  ?.copyWith(color: Theme.of(context).colorScheme.error),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 16),
            OutlinedButton(onPressed: onRetry, child: const Text('Retry')),
          ],
        ),
      ),
    );
  }
}
