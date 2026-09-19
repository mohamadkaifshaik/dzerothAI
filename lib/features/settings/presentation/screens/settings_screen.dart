import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:go_router/go_router.dart';

import '../bloc/settings_bloc.dart';

/// Account settings screen.
///
/// Allows the authenticated user to:
/// - toggle account privacy (private/public)
/// - initiate account suspension (behind a confirmation dialog)
///
/// No public social-validation metrics are displayed here.
class SettingsScreen extends StatefulWidget {
  const SettingsScreen({super.key});

  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  // Track whether at least one successful settings load has occurred.
  // Once true, errors are shown as snackbars and the interactive body
  // remains visible rather than replacing the content area.
  bool _hasLoaded = false;

  @override
  void initState() {
    super.initState();
    context.read<SettingsBloc>().add(const SettingsFetchRequested());
  }

  Future<void> _confirmSuspend(BuildContext context) async {
    final confirmed = await showDialog<bool>(
      context: context,
      barrierDismissible: false,
      builder: (dialogContext) => AlertDialog(
        title: const Text('Suspend your account?'),
        content: const Text(
          'Your account will be suspended. You will not be able to log back in.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(false),
            child: const Text('Cancel'),
          ),
          TextButton(
            style: TextButton.styleFrom(
              foregroundColor: Theme.of(dialogContext).colorScheme.error,
            ),
            onPressed: () => Navigator.of(dialogContext).pop(true),
            child: const Text('Confirm'),
          ),
        ],
      ),
    );

    if (confirmed == true && context.mounted) {
      context.read<SettingsBloc>().add(
        const SettingsAccountSuspensionRequested(),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Settings')),
      body: BlocConsumer<SettingsBloc, SettingsState>(
        listener: (context, state) {
          if (state is SettingsAccountSuspended) {
            // The user is now suspended — navigate to login. The auth guard
            // will handle clearing session state.
            context.go('/auth/login');
            return;
          }

          if (state is SettingsLoaded || state is SettingsSaved) {
            _hasLoaded = true;
          }

          if (state is SettingsError && _hasLoaded) {
            // After a successful initial load, errors surface as snackbars
            // so the interactive body stays visible.
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
          // Show spinner for initial states.
          if (state is SettingsInitial || state is SettingsLoading) {
            return const Center(child: CircularProgressIndicator());
          }

          // Show error body only when no data has ever been loaded.
          if (state is SettingsError && !_hasLoaded) {
            return _ErrorBody(
              message: state.failure.message,
              onRetry: () => context.read<SettingsBloc>().add(
                const SettingsFetchRequested(),
              ),
            );
          }

          return _SettingsBody(
            isPrivate: _resolvePrivacy(state),
            isSaving: state is SettingsSaving,
            onPrivacyChanged: (value) => context.read<SettingsBloc>().add(
              SettingsPrivacyToggled(isPrivate: value),
            ),
            onSuspendTap: () => _confirmSuspend(context),
          );
        },
      ),
    );
  }

  bool _resolvePrivacy(SettingsState state) {
    return switch (state) {
      SettingsLoaded(:final isPrivate) => isPrivate,
      SettingsSaving(:final isPrivate) => isPrivate,
      SettingsSaved(:final isPrivate) => isPrivate,
      _ => false,
    };
  }
}

// ---------------------------------------------------------------------------
// Private sub-widgets
// ---------------------------------------------------------------------------

class _SettingsBody extends StatelessWidget {
  const _SettingsBody({
    required this.isPrivate,
    required this.isSaving,
    required this.onPrivacyChanged,
    required this.onSuspendTap,
  });

  final bool isPrivate;
  final bool isSaving;
  final ValueChanged<bool> onPrivacyChanged;
  final VoidCallback onSuspendTap;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return ListView(
      children: [
        // Privacy section
        _SectionHeader(title: 'Privacy'),
        SwitchListTile(
          value: isPrivate,
          onChanged: isSaving ? null : onPrivacyChanged,
          title: const Text('Private Account'),
          subtitle: const Text(
            'When enabled, only approved followers can see your posts.',
          ),
          secondary: isSaving
              ? const SizedBox(
                  width: 24,
                  height: 24,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.lock_outline),
        ),
        const Divider(),

        // Account management section
        _SectionHeader(title: 'Account'),
        ListTile(
          leading: Icon(
            Icons.remove_circle_outline,
            color: theme.colorScheme.error,
          ),
          title: Text(
            'Suspend Account',
            style: TextStyle(color: theme.colorScheme.error),
          ),
          subtitle: const Text('This action will deactivate your account.'),
          onTap: onSuspendTap,
        ),
      ],
    );
  }
}

class _SectionHeader extends StatelessWidget {
  const _SectionHeader({required this.title});

  final String title;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 4),
      child: Text(
        title,
        style: theme.textTheme.labelLarge?.copyWith(
          color: theme.colorScheme.primary,
        ),
      ),
    );
  }
}

class _ErrorBody extends StatelessWidget {
  const _ErrorBody({required this.message, required this.onRetry});

  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.error_outline, size: 48, color: theme.colorScheme.error),
            const SizedBox(height: 16),
            Text(
              message,
              style: theme.textTheme.bodyLarge,
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            OutlinedButton(onPressed: onRetry, child: const Text('Retry')),
          ],
        ),
      ),
    );
  }
}
