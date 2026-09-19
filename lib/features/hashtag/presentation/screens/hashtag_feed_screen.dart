import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../post/presentation/widgets/go_touch_grass_widget.dart';
import '../../../post/presentation/widgets/post_card.dart';
import '../bloc/hashtag_feed_bloc.dart';

/// Displays a finite, cursor-paginated feed of posts for a given hashtag.
///
/// Per CLAUDE.md §2.1 there is NO infinite scrolling. The user must explicitly
/// press "Load More" to request the next page. When the feed is terminated
/// [GoTouchGrassWidget] is shown and no further fetching is permitted.
///
/// No social-validation metrics are rendered per CLAUDE.md §2.3.
class HashtagFeedScreen extends StatelessWidget {
  const HashtagFeedScreen({super.key, required this.tag});

  /// The hashtag to display (may include a leading '#').
  final String tag;

  String get _displayTag => tag.startsWith('#') ? tag : '#$tag';

  @override
  Widget build(BuildContext context) {
    return BlocBuilder<HashtagFeedBloc, HashtagFeedState>(
      builder: (context, state) {
        return switch (state) {
          HashtagFeedInitial() => _InitialView(tag: _displayTag),
          HashtagFeedLoading() => _LoadingView(tag: _displayTag),
          HashtagFeedLoaded() => _LoadedView(state: state, tag: _displayTag),
          HashtagFeedTerminated() => _TerminatedView(
            state: state,
            tag: _displayTag,
          ),
          HashtagFeedEmpty() => _EmptyView(tag: _displayTag),
          HashtagFeedError() => _ErrorView(state: state, tag: _displayTag),
        };
      },
    );
  }
}

// ---------------------------------------------------------------------------
// Initial state — trigger first load on mount.
// ---------------------------------------------------------------------------

class _InitialView extends StatefulWidget {
  const _InitialView({required this.tag});

  final String tag;

  @override
  State<_InitialView> createState() => _InitialViewState();
}

class _InitialViewState extends State<_InitialView> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) {
        context.read<HashtagFeedBloc>().add(const HashtagFeedLoadRequested());
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return _LoadingView(tag: widget.tag);
  }
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

class _LoadingView extends StatelessWidget {
  const _LoadingView({required this.tag});

  final String tag;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text(tag)),
      body: const Center(child: CircularProgressIndicator.adaptive()),
    );
  }
}

// ---------------------------------------------------------------------------
// Loaded
// ---------------------------------------------------------------------------

class _LoadedView extends StatelessWidget {
  const _LoadedView({required this.state, required this.tag});

  final HashtagFeedLoaded state;
  final String tag;

  bool get _hasFooter =>
      state.hasMore || (!state.hasMore && state.nextCursor != null);

  @override
  Widget build(BuildContext context) {
    final itemCount = state.posts.length + (_hasFooter ? 1 : 0);

    return Scaffold(
      appBar: AppBar(title: Text(tag)),
      body: ListView.separated(
        itemCount: itemCount,
        separatorBuilder: (_, _) => const Divider(height: 1),
        itemBuilder: (context, index) {
          if (index < state.posts.length) {
            return PostCard(post: state.posts[index]);
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
      return const Padding(
        padding: EdgeInsets.all(24),
        child: Center(child: CircularProgressIndicator.adaptive()),
      );
    }
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 24),
      child: Center(
        child: OutlinedButton(
          onPressed: () => context.read<HashtagFeedBloc>().add(
            const HashtagFeedNextPageRequested(),
          ),
          child: const Text('Load More'),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Terminated
// ---------------------------------------------------------------------------

class _TerminatedView extends StatelessWidget {
  const _TerminatedView({required this.state, required this.tag});

  final HashtagFeedTerminated state;
  final String tag;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text(tag)),
      body: ListView.separated(
        itemCount: state.posts.length + 1, // +1 for GoTouchGrassWidget
        separatorBuilder: (_, _) => const Divider(height: 1),
        itemBuilder: (context, index) {
          if (index < state.posts.length) {
            return PostCard(post: state.posts[index]);
          }
          return const GoTouchGrassWidget();
        },
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Empty
// ---------------------------------------------------------------------------

class _EmptyView extends StatelessWidget {
  const _EmptyView({required this.tag});

  final String tag;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(tag)),
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                Icons.tag,
                size: 64,
                color: theme.colorScheme.onSurfaceVariant,
              ),
              const SizedBox(height: 16),
              Text(
                'No posts with $tag yet.',
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
// Error
// ---------------------------------------------------------------------------

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.state, required this.tag});

  final HashtagFeedError state;
  final String tag;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(tag)),
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
              FilledButton(
                onPressed: () => context.read<HashtagFeedBloc>().add(
                  const HashtagFeedLoadRequested(),
                ),
                child: const Text('Retry'),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
