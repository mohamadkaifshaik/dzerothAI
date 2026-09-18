import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../bloc/post_feed_bloc.dart';
import 'post_card.dart';

/// Renders a paginated list of posts driven by a [PostFeedBloc].
///
/// Finite feed invariant (CLAUDE.md §2.1):
/// - When the BLoC is in [PostFeedTerminated] state, this widget shows the
///   complete list followed by the "Go Touch Grass" termination boundary.
/// - A "load more" affordance is shown only when [PostFeedLoaded.hasMore] is
///   true.  No infinite or hidden pagination is performed.
class PostThreadView extends StatelessWidget {
  const PostThreadView({super.key, this.onRetry});

  /// Called when the user taps the retry button in the error state.
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    return BlocBuilder<PostFeedBloc, PostFeedState>(
      builder: (context, state) {
        return switch (state) {
          PostFeedInitial() => const SizedBox.shrink(),
          PostFeedLoading() => const Center(
            child: Padding(
              padding: EdgeInsets.all(32),
              child: CircularProgressIndicator(),
            ),
          ),
          PostFeedLoaded(:final posts, :final hasMore) => _LoadedList(
            posts: posts,
            hasMore: hasMore,
          ),
          PostFeedTerminated(:final posts) => _TerminatedList(posts: posts),
          PostFeedError(:final failure) => _ErrorView(
            message: failure.message,
            onRetry: onRetry,
          ),
        };
      },
    );
  }
}

// ---------------------------------------------------------------------------
// Loaded list (more pages may exist)
// ---------------------------------------------------------------------------

class _LoadedList extends StatelessWidget {
  const _LoadedList({required this.posts, required this.hasMore});

  final List<dynamic> posts;
  final bool hasMore;

  @override
  Widget build(BuildContext context) {
    if (posts.isEmpty) {
      return const _EmptyView();
    }

    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        ListView.separated(
          shrinkWrap: true,
          physics: const NeverScrollableScrollPhysics(),
          itemCount: posts.length,
          separatorBuilder: (_, _) => const Divider(height: 1),
          itemBuilder: (context, index) => PostCard(post: posts[index]),
        ),
        if (hasMore)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 8),
            child: TextButton(
              onPressed: () => context.read<PostFeedBloc>().add(
                const PostFeedNextPageRequested(),
              ),
              child: const Text('Load more'),
            ),
          ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// Terminated list — CLAUDE.md §2.1 boundary state
// ---------------------------------------------------------------------------

class _TerminatedList extends StatelessWidget {
  const _TerminatedList({required this.posts});

  final List<dynamic> posts;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (posts.isNotEmpty)
          ListView.separated(
            shrinkWrap: true,
            physics: const NeverScrollableScrollPhysics(),
            itemCount: posts.length,
            separatorBuilder: (_, _) => const Divider(height: 1),
            itemBuilder: (context, index) => PostCard(post: posts[index]),
          ),
        const Divider(height: 1),
        // Feed termination boundary — "Go Touch Grass" experience.
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 32, horizontal: 24),
          child: Column(
            children: [
              Icon(
                Icons.grass_rounded,
                size: 48,
                color: theme.colorScheme.primary,
              ),
              const SizedBox(height: 12),
              Text(
                "You're all caught up!",
                style: theme.textTheme.titleMedium?.copyWith(
                  fontWeight: FontWeight.w600,
                ),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 8),
              Text(
                'Go touch grass.',
                style: theme.textTheme.bodyMedium?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
                textAlign: TextAlign.center,
              ),
            ],
          ),
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

class _EmptyView extends StatelessWidget {
  const _EmptyView();

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(32),
      child: Center(
        child: Text(
          'No posts yet.',
          style: Theme.of(context).textTheme.bodyLarge
              ?.copyWith(color: Theme.of(context).colorScheme.onSurfaceVariant),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Error state
// ---------------------------------------------------------------------------

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.message, this.onRetry});

  final String message;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    return Padding(
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
          if (onRetry != null) ...[
            const SizedBox(height: 16),
            OutlinedButton(onPressed: onRetry, child: const Text('Retry')),
          ],
        ],
      ),
    );
  }
}
