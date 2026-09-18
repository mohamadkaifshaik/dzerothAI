import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

/// Displays the feed-termination ("Go Touch Grass") experience.
///
/// This widget is shown when any feed reaches the server-enforced boundary
/// per CLAUDE.md §2.1.  It is visually distinct from an empty-feed state.
///
/// There is intentionally no "Load More" affordance: the feed is terminated
/// and no further fetching is permitted.
class GoTouchGrassWidget extends StatelessWidget {
  const GoTouchGrassWidget({super.key, this.onExplore});

  /// Optional callback invoked when the user presses the explore button.
  /// When null the button navigates to [/home/search].
  final VoidCallback? onExplore;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colorScheme = theme.colorScheme;

    return Semantics(
      label: "You've reached the end of your feed. Go touch grass.",
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 32, vertical: 48),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            // Divider to visually mark the feed boundary.
            Divider(color: colorScheme.outlineVariant),
            const SizedBox(height: 32),

            // Icon — leaf to reinforce the "go outside" message.
            Icon(
              Icons.eco_outlined,
              size: 72,
              color: colorScheme.primary,
              semanticLabel: '',
            ),
            const SizedBox(height: 20),

            Text(
              "You've reached the end.",
              style: theme.textTheme.titleMedium?.copyWith(
                fontWeight: FontWeight.w700,
                color: colorScheme.onSurface,
              ),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 8),
            Text(
              'Go touch grass.',
              style: theme.textTheme.bodyMedium?.copyWith(
                color: colorScheme.onSurfaceVariant,
              ),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 28),

            // Non-fetch call-to-action: navigate to search/explore.
            FilledButton.tonal(
              onPressed: onExplore ?? () => context.go('/home/search'),
              child: const Text('Explore Dzeroth'),
            ),
          ],
        ),
      ),
    );
  }
}
