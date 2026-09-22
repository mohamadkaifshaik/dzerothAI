import 'package:flutter/material.dart';

import '../../domain/entities/user_title.dart';

/// A small inline badge that renders a [TitleSummary.displayName].
///
/// Returns [SizedBox.shrink] when [title] is null so callers can
/// unconditionally place this widget without null-guard boilerplate.
///
/// PUBLIC SURFACE: Only [displayName] is rendered. [slug], category,
/// status, and is_revocable are NOT displayed per CLAUDE.md §2.3.
class TitleBadgeWidget extends StatelessWidget {
  const TitleBadgeWidget({super.key, required this.title});

  final TitleSummary? title;

  @override
  Widget build(BuildContext context) {
    final t = title;
    if (t == null) return const SizedBox.shrink();

    final theme = Theme.of(context);

    return Semantics(
      label: 'Title: ${t.displayName}',
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
        decoration: BoxDecoration(
          color: theme.colorScheme.primaryContainer,
          borderRadius: BorderRadius.circular(4),
        ),
        child: Text(
          t.displayName,
          style: theme.textTheme.labelSmall?.copyWith(
            color: theme.colorScheme.onPrimaryContainer,
            fontWeight: FontWeight.w600,
          ),
          overflow: TextOverflow.ellipsis,
        ),
      ),
    );
  }
}
