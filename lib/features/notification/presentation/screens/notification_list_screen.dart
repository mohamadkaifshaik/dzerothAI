import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../post/presentation/widgets/go_touch_grass_widget.dart';
import '../../domain/entities/notification.dart' as domain;
import '../bloc/notification_list_bloc.dart';

/// Displays the authenticated user's notification list.
///
/// This screen requires authentication — the router must guard it.
///
/// Per CLAUDE.md §2.1 there is no infinite scrolling.
/// A "Load more" button triggers the next page.
/// When the feed terminates, [GoTouchGrassWidget] is shown.
///
/// PUBLIC METRICS LOCKDOWN: No reaction counts, impression counts, bookmark
/// counts, or follower counts are displayed anywhere on this screen
/// (CLAUDE.md §2.3).
///
/// Mark-all-read fires only on explicit user action, never automatically on
/// screen open.
class NotificationListScreen extends StatefulWidget {
  const NotificationListScreen({super.key});

  @override
  State<NotificationListScreen> createState() => _NotificationListScreenState();
}

class _NotificationListScreenState extends State<NotificationListScreen> {
  @override
  void initState() {
    super.initState();
    context.read<NotificationListBloc>().add(
      const NotificationListFetchRequested(),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Notifications'),
        actions: [
          BlocBuilder<NotificationListBloc, NotificationListState>(
            builder: (context, state) {
              final hasNotifications =
                  state is NotificationListLoaded ||
                  state is NotificationListTerminated;
              if (!hasNotifications) return const SizedBox.shrink();
              return TextButton(
                onPressed: () => context.read<NotificationListBloc>().add(
                  const NotificationListMarkAllReadRequested(),
                ),
                child: const Text('Mark all read'),
              );
            },
          ),
        ],
      ),
      body: BlocBuilder<NotificationListBloc, NotificationListState>(
        builder: (context, state) {
          return switch (state) {
            NotificationListInitial() ||
            NotificationListLoading() => _buildLoading(),
            NotificationListLoaded(:final notifications, :final hasMore) =>
              _buildList(
                context,
                notifications: notifications,
                hasMore: hasMore,
                terminated: false,
              ),
            NotificationListTerminated(:final notifications) => _buildList(
              context,
              notifications: notifications,
              hasMore: false,
              terminated: true,
            ),
            NotificationListEmpty() => _buildEmpty(context),
            NotificationListError(:final failure) => _buildError(
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
              Icons.notifications_none,
              size: 64,
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
            const SizedBox(height: 16),
            Text(
              'No notifications yet.',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 8),
            Text(
              'When someone follows you, mentions you, or replies, '
              "you'll see it here.",
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
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
              onPressed: () => context.read<NotificationListBloc>().add(
                const NotificationListFetchRequested(),
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
    required List<domain.Notification> notifications,
    required bool hasMore,
    required bool terminated,
  }) {
    return ListView.separated(
      itemCount: notifications.length + 1,
      separatorBuilder: (_, _) => const Divider(height: 1),
      itemBuilder: (context, index) {
        if (index < notifications.length) {
          return _NotificationTile(notification: notifications[index]);
        }

        // Footer: termination widget or load-more affordance.
        if (terminated) {
          return const GoTouchGrassWidget();
        }

        return _buildLoadMoreButton(context, hasMore: hasMore);
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
          onPressed: () => context.read<NotificationListBloc>().add(
            const NotificationListNextPageRequested(),
          ),
          child: const Text('Load more'),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Notification tile
// ---------------------------------------------------------------------------

/// Renders a single notification row.
///
/// PUBLIC METRICS LOCKDOWN: deliberately exposes only event type, actor
/// handle, and optional post reference. No counts or metrics.
class _NotificationTile extends StatelessWidget {
  const _NotificationTile({required this.notification});

  final domain.Notification notification;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colorScheme = theme.colorScheme;

    return Semantics(
      label: _semanticLabel,
      child: Container(
        color: notification.isRead
            ? null
            : colorScheme.primaryContainer.withAlpha(51),
        child: ListTile(
          leading: CircleAvatar(
            radius: 22,
            backgroundImage: notification.actor.avatarUrl != null
                ? NetworkImage(notification.actor.avatarUrl!)
                : null,
            backgroundColor: colorScheme.surfaceContainerHighest,
            child: notification.actor.avatarUrl == null
                ? Text(
                    _avatarInitial,
                    style: theme.textTheme.titleSmall?.copyWith(
                      color: colorScheme.onSurfaceVariant,
                    ),
                  )
                : null,
          ),
          title: RichText(
            text: TextSpan(
              style: theme.textTheme.bodyMedium,
              children: [
                TextSpan(
                  text: '@${notification.actor.handle} ',
                  style: const TextStyle(fontWeight: FontWeight.w600),
                ),
                TextSpan(text: notification.event.label),
              ],
            ),
          ),
          subtitle: _buildSubtitle(context),
          trailing: notification.isRead
              ? null
              : Container(
                  width: 8,
                  height: 8,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: colorScheme.primary,
                  ),
                ),
        ),
      ),
    );
  }

  Widget? _buildSubtitle(BuildContext context) {
    if (notification.postId == null) return null;
    return Text(
      'View post',
      style: Theme.of(context).textTheme.bodySmall
          ?.copyWith(color: Theme.of(context).colorScheme.onSurfaceVariant),
    );
  }

  String get _avatarInitial {
    final name = notification.actor.displayName;
    return name.isNotEmpty ? name[0].toUpperCase() : '?';
  }

  String get _semanticLabel {
    return '@${notification.actor.handle} ${notification.event.label}'
        '${notification.isRead ? '' : ', unread'}';
  }
}
