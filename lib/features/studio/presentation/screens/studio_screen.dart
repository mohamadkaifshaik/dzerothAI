import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../post/presentation/widgets/go_touch_grass_widget.dart';
import '../../domain/entities/post_analytics.dart';
import '../bloc/studio_bloc.dart';

/// Private Creator Studio — shows post analytics for the authenticated author.
///
/// PRIVATE ANALYTICS: All engagement counts rendered here are private data
/// belonging exclusively to the authenticated post author. They must NOT appear
/// in any public-facing screen (feed, profile, search) per CLAUDE.md §2.3.
///
/// Per CLAUDE.md §2.1 there is no infinite scrolling. A "Load More" button
/// triggers the next page. When the feed terminates, [GoTouchGrassWidget] is
/// shown.
class StudioScreen extends StatefulWidget {
  const StudioScreen({super.key});

  @override
  State<StudioScreen> createState() => _StudioScreenState();
}

class _StudioScreenState extends State<StudioScreen> {
  @override
  void initState() {
    super.initState();
    context.read<StudioBloc>().add(const StudioFetchRequested());
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Creator Studio')),
      body: BlocBuilder<StudioBloc, StudioState>(
        builder: (context, state) {
          return switch (state) {
            StudioInitial() || StudioLoading() => _buildLoading(),
            StudioLoaded(:final items, :final hasMore) => _buildList(
              context,
              items: items,
              hasMore: hasMore,
              terminated: false,
            ),
            StudioTerminated(:final items) => _buildList(
              context,
              items: items,
              hasMore: false,
              terminated: true,
            ),
            StudioEmpty() => _buildEmpty(context),
            StudioError(:final failure) => _buildError(
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

  Widget _buildEmpty(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.bar_chart_outlined,
              size: 64,
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
            const SizedBox(height: 16),
            Text(
              'No posts yet.',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 8),
            Text(
              'Analytics will appear here once you publish posts.',
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
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
              onPressed: () =>
                  context.read<StudioBloc>().add(const StudioFetchRequested()),
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
    required List<PostAnalytics> items,
    required bool hasMore,
    required bool terminated,
  }) {
    if (items.isEmpty && !terminated) {
      return _buildEmpty(context);
    }

    return ListView.separated(
      itemCount: items.length + (terminated || !hasMore ? 1 : 2),
      separatorBuilder: (context, index) => const Divider(height: 1),
      itemBuilder: (context, index) {
        if (index < items.length) {
          return _AnalyticsCard(analytics: items[index]);
        }

        // After the list items: termination widget or load-more button.
        if (terminated) {
          return const GoTouchGrassWidget();
        }

        // hasMore is false while request is in-flight; show a loading indicator.
        if (index == items.length) {
          return _buildLoadMoreButton(context, hasMore: hasMore);
        }

        return const SizedBox.shrink();
      },
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
          onPressed: () =>
              context.read<StudioBloc>().add(const StudioNextPageRequested()),
          child: const Text('Load more'),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Analytics card
// ---------------------------------------------------------------------------

/// Renders private analytics for a single post.
///
/// PRIVATE: The counts displayed here are only visible to the authenticated
/// post author inside Creator Studio. Never reuse this widget in public screens.
class _AnalyticsCard extends StatelessWidget {
  const _AnalyticsCard({required this.analytics});

  final PostAnalytics analytics;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colorScheme = theme.colorScheme;

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Post type badge
          Row(
            children: [
              _PostTypeBadge(postType: analytics.postType),
              const Spacer(),
              Text(
                _formatDate(analytics.createdAt),
                style: theme.textTheme.bodySmall?.copyWith(
                  color: colorScheme.onSurfaceVariant,
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),

          // Content preview
          Text(
            analytics.content,
            style: theme.textTheme.bodyMedium,
            maxLines: 3,
            overflow: TextOverflow.ellipsis,
          ),
          const SizedBox(height: 12),

          // Private engagement metrics (4-column row)
          // These are ONLY shown to the post author in Creator Studio.
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceEvenly,
            children: [
              _MetricCell(
                icon: Icons.favorite_outline,
                label: 'Reactions',
                count: analytics.reactionCount,
                color: colorScheme.primary,
              ),
              _MetricCell(
                icon: Icons.bookmark_outline,
                label: 'Bookmarks',
                count: analytics.bookmarkCount,
                color: colorScheme.secondary,
              ),
              _MetricCell(
                icon: Icons.reply_outlined,
                label: 'Replies',
                count: analytics.replyCount,
                color: colorScheme.tertiary,
              ),
              _MetricCell(
                icon: Icons.format_quote_outlined,
                label: 'Quotes',
                count: analytics.quoteCount,
                color: colorScheme.onSurfaceVariant,
              ),
            ],
          ),
        ],
      ),
    );
  }

  String _formatDate(DateTime dt) {
    const months = [
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
    return '${months[dt.month - 1]} ${dt.day}, ${dt.year}';
  }
}

class _PostTypeBadge extends StatelessWidget {
  const _PostTypeBadge({required this.postType});

  final String postType;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: theme.colorScheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        postType,
        style: theme.textTheme.labelSmall?.copyWith(
          color: theme.colorScheme.onSurfaceVariant,
        ),
      ),
    );
  }
}

class _MetricCell extends StatelessWidget {
  const _MetricCell({
    required this.icon,
    required this.label,
    required this.count,
    required this.color,
  });

  final IconData icon;
  final String label;
  final int count;
  final Color color;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Semantics(
      label: '$count $label',
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 20, color: color),
          const SizedBox(height: 2),
          Text(
            '$count',
            style: theme.textTheme.labelLarge?.copyWith(
              fontWeight: FontWeight.w700,
            ),
          ),
          Text(
            label,
            style: theme.textTheme.labelSmall?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
            ),
          ),
        ],
      ),
    );
  }
}
