import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../domain/repositories/report_repository.dart';
import '../bloc/report_bloc.dart';

/// Identifies whether the report targets a post or a user.
enum _ReportTarget { post, user }

/// A modal bottom sheet that collects a reason (and optional detail) before
/// submitting a post or user report.
///
/// Usage — post:
/// ```dart
/// showModalBottomSheet(
///   context: context,
///   isScrollControlled: true,
///   builder: (_) => ReportSheet.forPost(
///     postId: post.id,
///     reportRepository: repository,
///   ),
/// );
/// ```
///
/// Usage — user:
/// ```dart
/// showModalBottomSheet(
///   context: context,
///   isScrollControlled: true,
///   builder: (_) => ReportSheet.forUser(
///     userId: userId,
///     reportRepository: repository,
///   ),
/// );
/// ```
///
/// The sheet creates its own [ReportBloc] instance via [BlocProvider] so no
/// external BLoC wiring is required.  The sheet closes automatically on
/// [ReportSuccess] and shows a snackbar on the parent's [ScaffoldMessenger].
/// On [ReportError] the error message is displayed inside the sheet.
///
/// PRIVACY: Reporter identity is never displayed.
/// PUBLIC METRICS LOCKDOWN: no social-validation metrics are rendered here.
class ReportSheet extends StatelessWidget {
  const ReportSheet._({
    required this.reportRepository,
    required this._target,
    required this.targetId,
  });

  /// Opens a [ReportSheet] pre-configured to report a post.
  factory ReportSheet.forPost({
    required String postId,
    required ReportRepository reportRepository,
  }) {
    return ReportSheet._(
      reportRepository: reportRepository,
      target: _ReportTarget.post,
      targetId: postId,
    );
  }

  /// Opens a [ReportSheet] pre-configured to report a user.
  factory ReportSheet.forUser({
    required String userId,
    required ReportRepository reportRepository,
  }) {
    return ReportSheet._(
      reportRepository: reportRepository,
      target: _ReportTarget.user,
      targetId: userId,
    );
  }

  final ReportRepository reportRepository;
  final _ReportTarget _target;
  final String targetId;

  @override
  Widget build(BuildContext context) {
    return BlocProvider<ReportBloc>(
      create: (_) => ReportBloc(reportRepository: reportRepository),
      child: _ReportSheetBody(target: _target, targetId: targetId),
    );
  }
}

// ---------------------------------------------------------------------------
// Sheet body (stateful to hold form state)
// ---------------------------------------------------------------------------

class _ReportSheetBody extends StatefulWidget {
  const _ReportSheetBody({required this.target, required this.targetId});

  final _ReportTarget target;
  final String targetId;

  @override
  State<_ReportSheetBody> createState() => _ReportSheetBodyState();
}

class _ReportSheetBodyState extends State<_ReportSheetBody> {
  static const _reasons = <_ReasonOption>[
    _ReasonOption(code: 'spam', label: 'Spam'),
    _ReasonOption(code: 'harassment', label: 'Harassment'),
    _ReasonOption(code: 'misinformation', label: 'Misinformation'),
    _ReasonOption(code: 'hate_speech', label: 'Hate speech'),
    _ReasonOption(code: 'violence', label: 'Violence'),
    _ReasonOption(code: 'other', label: 'Other'),
  ];

  static const int _detailMaxLength = 500;

  String? _selectedReason;
  final TextEditingController _detailController = TextEditingController();
  final GlobalKey<FormState> _formKey = GlobalKey<FormState>();

  @override
  void dispose() {
    _detailController.dispose();
    super.dispose();
  }

  void _submit(BuildContext context) {
    if (_selectedReason == null) return;
    if (!(_formKey.currentState?.validate() ?? true)) return;
    final detail = _detailController.text.trim();
    final event = widget.target == _ReportTarget.post
        ? ReportPostSubmitted(
            postId: widget.targetId,
            reason: _selectedReason!,
            detail: detail.isEmpty ? null : detail,
          )
        : ReportUserSubmitted(
            userId: widget.targetId,
            reason: _selectedReason!,
            detail: detail.isEmpty ? null : detail,
          );
    context.read<ReportBloc>().add(event);
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final title = widget.target == _ReportTarget.post
        ? 'Report post'
        : 'Report user';

    return BlocConsumer<ReportBloc, ReportState>(
      listener: (context, state) {
        if (state is ReportSuccess) {
          // Close sheet first, then show snackbar on the parent scaffold.
          Navigator.of(context).pop();
          ScaffoldMessenger.of(context)
              .showSnackBar(const SnackBar(content: Text('Report submitted')));
        }
      },
      builder: (context, state) {
        final isLoading = state is ReportLoading;

        return Padding(
          padding: EdgeInsets.only(
            left: 16,
            right: 16,
            top: 16,
            bottom: MediaQuery.of(context).viewInsets.bottom + 24,
          ),
          child: Form(
            key: _formKey,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                // Handle bar
                Center(
                  child: Container(
                    width: 40,
                    height: 4,
                    margin: const EdgeInsets.only(bottom: 16),
                    decoration: BoxDecoration(
                      color: theme.colorScheme.onSurfaceVariant.withValues(
                        alpha: 0.4,
                      ),
                      borderRadius: BorderRadius.circular(2),
                    ),
                  ),
                ),

                Text(
                  title,
                  style: theme.textTheme.titleLarge,
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: 16),

                // Error banner (visible only in ReportError state)
                if (state is ReportError) ...[
                  _ErrorBanner(message: state.failure.message),
                  const SizedBox(height: 12),
                ],

                // Reason list
                ..._reasons.map(
                  // ignore: deprecated_member_use
                  (option) => RadioListTile<String>(
                    value: option.code,
                    // ignore: deprecated_member_use
                    groupValue: _selectedReason,
                    title: Text(option.label),
                    // ignore: deprecated_member_use
                    onChanged: isLoading
                        ? null
                        : (value) => setState(() => _selectedReason = value),
                    contentPadding: EdgeInsets.zero,
                  ),
                ),

                const SizedBox(height: 8),

                // Optional detail field
                TextFormField(
                  controller: _detailController,
                  enabled: !isLoading,
                  maxLength: _detailMaxLength,
                  maxLines: 3,
                  decoration: const InputDecoration(
                    labelText: 'Additional details (optional)',
                    hintText: 'Provide any extra context…',
                    border: OutlineInputBorder(),
                  ),
                  validator: (value) {
                    if (value != null && value.length > _detailMaxLength) {
                      return 'Detail must not exceed $_detailMaxLength characters.';
                    }
                    return null;
                  },
                ),

                const SizedBox(height: 16),

                // Submit button
                FilledButton(
                  onPressed: isLoading || _selectedReason == null
                      ? null
                      : () => _submit(context),
                  child: isLoading
                      ? const SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: Colors.white,
                          ),
                        )
                      : const Text('Submit report'),
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}

// ---------------------------------------------------------------------------
// Error banner
// ---------------------------------------------------------------------------

class _ErrorBanner extends StatelessWidget {
  const _ErrorBanner({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: Theme.of(context).colorScheme.errorContainer,
        borderRadius: BorderRadius.circular(8),
      ),
      child: Row(
        children: [
          Icon(
            Icons.error_outline,
            size: 18,
            color: Theme.of(context).colorScheme.onErrorContainer,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              message,
              style: Theme.of(context).textTheme.bodySmall?.copyWith(
                color: Theme.of(context).colorScheme.onErrorContainer,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Internal data class
// ---------------------------------------------------------------------------

class _ReasonOption {
  const _ReasonOption({required this.code, required this.label});

  final String code;
  final String label;
}
