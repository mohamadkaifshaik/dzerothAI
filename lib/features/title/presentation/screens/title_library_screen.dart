import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../domain/entities/user_title.dart';
import '../bloc/title_library_bloc.dart';
import '../widgets/title_badge_widget.dart';

/// Authenticated screen for managing the user's title library.
///
/// Shows all owned titles, indicates which is currently primary, and lets
/// the user set or clear the primary title.
///
/// PRIVATE MANAGEMENT SCREEN: [UserTitle.category], [UserTitle.status], and
/// [UserTitle.isRevocable] are displayed here because this is a private
/// management surface. They must NOT appear on any public feed, profile card,
/// or post surface per CLAUDE.md §2.3.
class TitleLibraryScreen extends StatefulWidget {
  const TitleLibraryScreen({super.key});

  @override
  State<TitleLibraryScreen> createState() => _TitleLibraryScreenState();
}

class _TitleLibraryScreenState extends State<TitleLibraryScreen> {
  @override
  void initState() {
    super.initState();
    context.read<TitleLibraryBloc>().add(const LoadTitleLibrary());
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('My Titles')),
      body: BlocConsumer<TitleLibraryBloc, TitleLibraryState>(
        listener: (context, state) {
          if (state is TitleLibraryError) {
            ScaffoldMessenger.of(context)
              ..hideCurrentSnackBar()
              ..showSnackBar(
                SnackBar(
                  content: Text(state.failure.message),
                  backgroundColor: Theme.of(context).colorScheme.error,
                ),
              );
          }
        },
        builder: (context, state) {
          return switch (state) {
            TitleLibraryInitial() ||
            TitleLibraryLoading() => _buildLoading(),
            TitleLibraryLoaded(:final titles, :final primaryId) =>
              _buildBody(context, titles: titles, primaryId: primaryId),
            // Mutation in-flight: show the existing titles (with the OLD
            // confirmed primaryId) plus a linear progress indicator at the top.
            // The primaryId is NOT updated until the server confirms the change.
            TitleLibraryMutating(:final titles, :final primaryId) =>
              _buildMutating(context, titles: titles, primaryId: primaryId),
            TitleLibraryError(:final failure) => _buildError(
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

  /// Renders the current title list with a linear progress bar at the top
  /// while a set/clear mutation is in-flight.
  ///
  /// The primaryId shown here is the OLD server-confirmed value — it does NOT
  /// change until getMyTitles confirms the new selection from the backend.
  Widget _buildMutating(
    BuildContext context, {
    required List<UserTitle> titles,
    required String? primaryId,
  }) {
    return Column(
      children: [
        const LinearProgressIndicator(),
        Expanded(
          child: _buildBody(context, titles: titles, primaryId: primaryId),
        ),
      ],
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
              onPressed: () => context.read<TitleLibraryBloc>().add(
                const LoadTitleLibrary(),
              ),
              icon: const Icon(Icons.refresh),
              label: const Text('Try again'),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildBody(
    BuildContext context, {
    required List<UserTitle> titles,
    required String? primaryId,
  }) {
    if (titles.isEmpty) {
      return _buildEmpty(context);
    }

    return ListView.separated(
      itemCount: titles.length + 1,
      separatorBuilder: (context, index) => const Divider(height: 1),
      itemBuilder: (context, index) {
        if (index == 0) {
          return _PrimaryTitleHeader(primaryId: primaryId, titles: titles);
        }
        final title = titles[index - 1];
        return _TitleRow(
          title: title,
          isPrimary: title.id == primaryId,
          onSetPrimary: () => context.read<TitleLibraryBloc>().add(
            SetPrimaryTitle(title.id),
          ),
          onClearPrimary: () => context.read<TitleLibraryBloc>().add(
            const ClearPrimaryTitle(),
          ),
        );
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
              Icons.workspace_premium_outlined,
              size: 64,
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
            const SizedBox(height: 16),
            Text(
              'No titles yet.',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 8),
            Text(
              'Titles you earn will appear here.',
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
}

// ---------------------------------------------------------------------------
// Primary title header summary
// ---------------------------------------------------------------------------

class _PrimaryTitleHeader extends StatelessWidget {
  const _PrimaryTitleHeader({
    required this.primaryId,
    required this.titles,
  });

  final String? primaryId;
  final List<UserTitle> titles;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final primaryTitle = primaryId != null
        ? titles.where((t) => t.id == primaryId).firstOrNull
        : null;

    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Primary Title',
                  style: theme.textTheme.labelLarge?.copyWith(
                    color: theme.colorScheme.primary,
                  ),
                ),
                const SizedBox(height: 4),
                if (primaryTitle != null)
                  TitleBadgeWidget(
                    title: primaryTitle.toSummary(),
                  )
                else
                  Text(
                    'None set',
                    style: theme.textTheme.bodyMedium?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Title row
// ---------------------------------------------------------------------------

class _TitleRow extends StatelessWidget {
  const _TitleRow({
    required this.title,
    required this.isPrimary,
    required this.onSetPrimary,
    required this.onClearPrimary,
  });

  final UserTitle title;
  final bool isPrimary;
  final VoidCallback onSetPrimary;
  final VoidCallback onClearPrimary;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return ListTile(
      leading: isPrimary
          ? Icon(Icons.star, color: theme.colorScheme.primary)
          : const Icon(Icons.star_border_outlined),
      title: Text(title.displayName),
      subtitle: Text(
        title.category.isNotEmpty ? title.category : title.status,
        style: theme.textTheme.bodySmall?.copyWith(
          color: theme.colorScheme.onSurfaceVariant,
        ),
      ),
      trailing: isPrimary
          ? TextButton(
              onPressed: onClearPrimary,
              child: const Text('Remove'),
            )
          : TextButton(
              onPressed: onSetPrimary,
              child: const Text('Set as primary'),
            ),
    );
  }
}

// ---------------------------------------------------------------------------
// Extension to convert UserTitle → TitleSummary for the badge
// ---------------------------------------------------------------------------

extension on UserTitle {
  TitleSummary toSummary() => TitleSummary(slug: slug, displayName: displayName);
}
