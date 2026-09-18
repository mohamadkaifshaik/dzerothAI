import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../bookmark/domain/repositories/bookmark_repository.dart';
import '../../../bookmark/presentation/bloc/bookmark_toggle_bloc.dart';
import '../../../post/presentation/widgets/go_touch_grass_widget.dart';
import '../../../post/presentation/widgets/post_card.dart';
import '../bloc/home_feed_bloc.dart';

/// The authenticated user's home timeline screen.
///
/// Renders a finite, cursor-paginated feed of posts.
///
/// Per CLAUDE.md §2.1 there is NO infinite scrolling.  The user must
/// explicitly press "Load More" to request the next page.  When the feed is
/// terminated the "Load More" button is hidden and [GoTouchGrassWidget] is
/// shown instead.
///
/// No social-validation metrics (likes, impressions, bookmarks, follower
/// counts, etc.) are rendered here or in [PostCard] per CLAUDE.md §2.3.
///
/// [bookmarkRepositoryFactory] is required so that each [PostCard] in the list
/// can be paired with its own [BookmarkToggleBloc] keyed by post ID.
class HomeFeedScreen extends StatelessWidget {
  const HomeFeedScreen({super.key, required this.bookmarkRepositoryFactory});

  final BookmarkRepository Function() bookmarkRepositoryFactory;

  @override
  Widget build(BuildContext context) {
    return BlocBuilder<HomeFeedBloc, HomeFeedState>(
      builder: (context, state) {
        return switch (state) {
          HomeFeedInitial() => _InitialView(
            onLoad: () =>
                context.read<HomeFeedBloc>().add(const HomeFeedLoadRequested()),
          ),
          HomeFeedLoading() => const _LoadingView(),
          HomeFeedLoaded() => _LoadedView(
            state: state,
            bookmarkRepositoryFactory: bookmarkRepositoryFactory,
          ),
          HomeFeedTerminated() => _TerminatedView(
            state: state,
            bookmarkRepositoryFactory: bookmarkRepositoryFactory,
          ),
          HomeFeedEmpty() => const _EmptyView(),
          HomeFeedError() => _ErrorView(
            state: state,
            onRetry: () =>
                context.read<HomeFeedBloc>().add(const HomeFeedLoadRequested()),
          ),
        };
      },
    );
  }
}

// ---------------------------------------------------------------------------
// Initial state — trigger first load immediately.
// ---------------------------------------------------------------------------

class _InitialView extends StatefulWidget {
  const _InitialView({required this.onLoad});

  final VoidCallback onLoad;

  @override
  State<_InitialView> createState() => _InitialViewState();
}

class _InitialViewState extends State<_InitialView> {
  @override
  void initState() {
    super.initState();
    // Dispatch the load on the next frame so the BLoC is ready.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) widget.onLoad();
    });
  }

  @override
  Widget build(BuildContext context) {
    return const _LoadingView();
  }
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

class _LoadingView extends StatelessWidget {
  const _LoadingView();

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Feed')),
      body: const Center(child: CircularProgressIndicator.adaptive()),
    );
  }
}

// ---------------------------------------------------------------------------
// Loaded — posts visible, "Load More" button when hasMore is true, or a
// spinner when a next-page request is in-flight (hasMore == false but
// nextCursor is still present).
// ---------------------------------------------------------------------------

class _LoadedView extends StatelessWidget {
  const _LoadedView({
    required this.state,
    required this.bookmarkRepositoryFactory,
  });

  final HomeFeedLoaded state;
  final BookmarkRepository Function() bookmarkRepositoryFactory;

  /// Whether to render a footer widget at all.
  bool get _hasFooter =>
      state.hasMore || (!state.hasMore && state.nextCursor != null);

  @override
  Widget build(BuildContext context) {
    final itemCount = state.posts.length + (_hasFooter ? 1 : 0);

    return Scaffold(
      appBar: AppBar(title: const Text('Feed')),
      body: ListView.separated(
        itemCount: itemCount,
        separatorBuilder: (context, index) => const Divider(height: 1),
        itemBuilder: (context, index) {
          if (index < state.posts.length) {
            final post = state.posts[index];
            return BlocProvider<BookmarkToggleBloc>(
              key: ValueKey('bookmark_${post.id}'),
              create: (_) => BookmarkToggleBloc(
                bookmarkRepository: bookmarkRepositoryFactory(),
                postId: post.id,
              ),
              child: PostCard(post: post),
            );
          }
          return _LoadMoreFooter(hasMore: state.hasMore);
        },
      ),
    );
  }
}

class _LoadMoreFooter extends StatelessWidget {
  const _LoadMoreFooter({required this.hasMore});

  final bool hasMore;

  @override
  Widget build(BuildContext context) {
    if (!hasMore) {
      // hasMore is false while the next-page request is in-flight.
      return const Padding(
        padding: EdgeInsets.all(24),
        child: Center(child: CircularProgressIndicator.adaptive()),
      );
    }
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 24),
      child: Center(
        child: OutlinedButton(
          onPressed: () => context.read<HomeFeedBloc>().add(
            const HomeFeedNextPageRequested(),
          ),
          child: const Text('Load More'),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Terminated — all posts shown + GoTouchGrassWidget.
// ---------------------------------------------------------------------------

class _TerminatedView extends StatelessWidget {
  const _TerminatedView({
    required this.state,
    required this.bookmarkRepositoryFactory,
  });

  final HomeFeedTerminated state;
  final BookmarkRepository Function() bookmarkRepositoryFactory;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Feed')),
      body: ListView.separated(
        itemCount: state.posts.length + 1, // +1 for GoTouchGrassWidget
        separatorBuilder: (context, index) => const Divider(height: 1),
        itemBuilder: (context, index) {
          if (index < state.posts.length) {
            final post = state.posts[index];
            return BlocProvider<BookmarkToggleBloc>(
              key: ValueKey('bookmark_${post.id}'),
              create: (_) => BookmarkToggleBloc(
                bookmarkRepository: bookmarkRepositoryFactory(),
                postId: post.id,
              ),
              child: PostCard(post: post),
            );
          }
          return const GoTouchGrassWidget();
        },
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Empty — user follows no one.
// ---------------------------------------------------------------------------

class _EmptyView extends StatelessWidget {
  const _EmptyView();

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(title: const Text('Feed')),
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                Icons.people_outline,
                size: 64,
                color: theme.colorScheme.onSurfaceVariant,
              ),
              const SizedBox(height: 16),
              Text(
                'Follow people to see their posts here.',
                style: theme.textTheme.bodyLarge?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
                textAlign: TextAlign.center,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Error — show message and retry button.
// ---------------------------------------------------------------------------

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.state, required this.onRetry});

  final HomeFeedError state;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(title: const Text('Feed')),
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                Icons.error_outline,
                size: 64,
                color: theme.colorScheme.error,
              ),
              const SizedBox(height: 16),
              Text(
                state.failure.message,
                style: theme.textTheme.bodyLarge?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 24),
              FilledButton(onPressed: onRetry, child: const Text('Retry')),
            ],
          ),
        ),
      ),
    );
  }
}
