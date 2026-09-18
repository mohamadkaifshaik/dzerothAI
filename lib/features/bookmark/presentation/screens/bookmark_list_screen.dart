import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../post/presentation/widgets/go_touch_grass_widget.dart';
import '../../../post/presentation/widgets/post_card.dart';
import '../bloc/bookmark_list_bloc.dart';

/// Displays the authenticated user's bookmarked posts.
///
/// This screen requires authentication — the router must guard it.
///
/// Per CLAUDE.md §2.1 there is no infinite scrolling.
/// A "Load More" button triggers the next page.
/// When the feed terminates, [GoTouchGrassWidget] is shown.
///
/// PUBLIC METRICS LOCKDOWN: No bookmark counts are displayed anywhere on this
/// screen per CLAUDE.md §2.3.
class BookmarkListScreen extends StatefulWidget {
  const BookmarkListScreen({super.key});

  @override
  State<BookmarkListScreen> createState() => _BookmarkListScreenState();
}

class _BookmarkListScreenState extends State<BookmarkListScreen> {
  @override
  void initState() {
    super.initState();
    context.read<BookmarkListBloc>().add(const BookmarkListLoadRequested());
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Bookmarks')),
      body: BlocBuilder<BookmarkListBloc, BookmarkListState>(
        builder: (context, state) {
          return switch (state) {
            BookmarkListInitial() || BookmarkListLoading() => _buildLoading(),
            BookmarkListLoaded(:final posts, :final hasMore) => _buildList(
              context,
              posts: posts,
              hasMore: hasMore,
              terminated: false,
            ),
            BookmarkListTerminated(:final posts) => _buildList(
              context,
              posts: posts,
              hasMore: false,
              terminated: true,
            ),
            BookmarkListError(:final failure) => _buildError(
              context,
              failure.message,
            ),
          };
        },
      ),
    );
  }

  Widget _buildLoading() {
    return const Center(child: CircularProgressIndicator());
  }

  Widget _buildError(BuildContext context, String message) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.error_outline,
              size: 48,
              color: Theme.of(context).colorScheme.error,
            ),
            const SizedBox(height: 16),
            Text(
              message,
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyLarge,
            ),
            const SizedBox(height: 24),
            OutlinedButton.icon(
              onPressed: () => context.read<BookmarkListBloc>().add(
                const BookmarkListLoadRequested(),
              ),
              icon: const Icon(Icons.refresh),
              label: const Text('Try again'),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildList(
    BuildContext context, {
    required List posts,
    required bool hasMore,
    required bool terminated,
  }) {
    if (posts.isEmpty && !terminated) {
      return _buildEmpty(context);
    }

    return ListView.separated(
      itemCount: posts.length + (terminated || !hasMore ? 1 : 2),
      separatorBuilder: (context, index) => const Divider(height: 1),
      itemBuilder: (context, index) {
        if (index < posts.length) {
          return PostCard(post: posts[index] as dynamic);
        }

        // After the list items: termination widget or load-more button.
        if (terminated) {
          return const GoTouchGrassWidget();
        }

        // hasMore is false while request in-flight; show a loading indicator.
        if (index == posts.length) {
          return _buildLoadMoreButton(context, hasMore: hasMore);
        }

        return const SizedBox.shrink();
      },
    );
  }

  Widget _buildEmpty(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.bookmark_border,
              size: 64,
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
            const SizedBox(height: 16),
            Text(
              'No bookmarks yet.',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 8),
            Text(
              'Save posts to read them later.',
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildLoadMoreButton(BuildContext context, {required bool hasMore}) {
    if (!hasMore) {
      return const Padding(
        padding: EdgeInsets.symmetric(vertical: 16),
        child: Center(child: CircularProgressIndicator()),
      );
    }

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 16),
      child: Center(
        child: OutlinedButton(
          onPressed: () => context.read<BookmarkListBloc>().add(
            const BookmarkListNextPageRequested(),
          ),
          child: const Text('Load more'),
        ),
      ),
    );
  }
}
