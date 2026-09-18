import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../bloc/post_compose_bloc.dart';

/// Screen for composing and submitting a new post.
///
/// Supports post types: 'original', 'reply', 'quote', 'repost'.
///
/// Per CLAUDE.md §2.2:
/// - Quote and repost actions require a mandatory 5-second countdown before
///   submission.  The countdown is displayed prominently and cannot be bypassed.
/// - Quote posts must contain at least 5 distinct words (enforced client-side
///   as a UX guard; the backend is authoritative).
///
/// Content limit: 500 Unicode code points (matches backend).
class PostComposeScreen extends StatefulWidget {
  const PostComposeScreen({
    super.key,
    this.postType = 'original',
    this.parentId,
    this.quotedPostId,
  });

  final String postType;
  final String? parentId;
  final String? quotedPostId;

  @override
  State<PostComposeScreen> createState() => _PostComposeScreenState();
}

class _PostComposeScreenState extends State<PostComposeScreen> {
  final _contentController = TextEditingController();
  int _runeCount = 0;

  static const int _maxRunes = PostComposeBloc.maxContentRunes;

  @override
  void initState() {
    super.initState();
    _contentController.addListener(_onTextChanged);
  }

  void _onTextChanged() {
    setState(() {
      _runeCount = _contentController.text.runes.length;
    });
  }

  @override
  void dispose() {
    _contentController.dispose();
    super.dispose();
  }

  bool get _isRepost => widget.postType == 'repost';

  bool get _canSubmit {
    if (_isRepost) return true;
    if (_runeCount == 0 || _runeCount > _maxRunes) return false;
    return true;
  }

  void _submit(BuildContext context) {
    final content = _isRepost ? null : _contentController.text.trim();
    context.read<PostComposeBloc>().add(
      PostComposeSubmitted(
        postType: widget.postType,
        content: content,
        parentId: widget.parentId,
        quotedPostId: widget.quotedPostId,
      ),
    );
  }

  void _cancelCountdown(BuildContext context) {
    context.read<PostComposeBloc>().add(const PostComposeCountdownCancelled());
  }

  @override
  Widget build(BuildContext context) {
    return BlocListener<PostComposeBloc, PostComposeState>(
      listener: (context, state) {
        if (state is PostComposeSuccess) {
          // Navigate to the newly created post's detail screen.
          Navigator.of(context).pop();
          // The parent screen should refresh; navigation side effects are
          // handled by the caller via BlocListener.
        } else if (state is PostComposeError) {
          ScaffoldMessenger.of(context)
              .showSnackBar(SnackBar(content: Text(state.failure.message)));
        }
      },
      child: Scaffold(
        appBar: AppBar(
          title: Text(_appBarTitle()),
          actions: [
            BlocBuilder<PostComposeBloc, PostComposeState>(
              builder: (context, state) {
                if (state is PostComposeCountdown) {
                  return _CountdownAction(
                    secondsRemaining: state.secondsRemaining,
                    onCancel: () => _cancelCountdown(context),
                  );
                }

                final isSubmitting = state is PostComposeSubmitting;
                return Padding(
                  padding: const EdgeInsets.only(right: 8),
                  child: FilledButton(
                    onPressed: (!_canSubmit || isSubmitting)
                        ? null
                        : () => _submit(context),
                    child: isSubmitting
                        ? const SizedBox(
                            width: 16,
                            height: 16,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : const Text('Post'),
                  ),
                );
              },
            ),
          ],
        ),
        body: BlocBuilder<PostComposeBloc, PostComposeState>(
          builder: (context, state) {
            final isCountingDown = state is PostComposeCountdown;
            return Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (widget.postType == 'quote') ...[
                    _QuoteNotice(),
                    const SizedBox(height: 12),
                  ],
                  if (isCountingDown) ...[
                    _CountdownBanner(
                      secondsRemaining: state.secondsRemaining,
                      totalSeconds: state.totalSeconds,
                      onCancel: () => _cancelCountdown(context),
                    ),
                  ] else if (!_isRepost) ...[
                    TextField(
                      controller: _contentController,
                      autofocus: true,
                      maxLines: null,
                      keyboardType: TextInputType.multiline,
                      textCapitalization: TextCapitalization.sentences,
                      decoration: InputDecoration(
                        hintText: _hintText(),
                        border: InputBorder.none,
                        enabledBorder: InputBorder.none,
                        focusedBorder: InputBorder.none,
                        fillColor: Colors.transparent,
                      ),
                      style: Theme.of(context).textTheme.bodyLarge,
                    ),
                    const Spacer(),
                    _CharacterCounter(
                      runeCount: _runeCount,
                      maxRunes: _maxRunes,
                    ),
                  ] else
                    Expanded(
                      child: Center(
                        child: Text(
                          'Repost this post?',
                          style: Theme.of(context).textTheme.bodyLarge,
                        ),
                      ),
                    ),
                ],
              ),
            );
          },
        ),
      ),
    );
  }

  String _appBarTitle() => switch (widget.postType) {
    'reply' => 'Reply',
    'quote' => 'Quote',
    'repost' => 'Repost',
    _ => 'New Post',
  };

  String _hintText() => switch (widget.postType) {
    'reply' => 'Write your reply...',
    'quote' => 'Add a comment (at least 5 distinct words)...',
    _ => "What's happening?",
  };
}

// ---------------------------------------------------------------------------
// Countdown banner — shown in the body during countdown
// ---------------------------------------------------------------------------

/// Full-screen body countdown banner displayed during the mandatory 5-second
/// wait for quote/repost actions (CLAUDE.md §2.2).
class _CountdownBanner extends StatelessWidget {
  const _CountdownBanner({
    required this.secondsRemaining,
    required this.totalSeconds,
    required this.onCancel,
  });

  final int secondsRemaining;
  final int totalSeconds;
  final VoidCallback onCancel;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Expanded(
      child: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              '$secondsRemaining',
              style: theme.textTheme.displayLarge?.copyWith(
                fontWeight: FontWeight.bold,
                color: theme.colorScheme.primary,
              ),
            ),
            const SizedBox(height: 12),
            Text(
              secondsRemaining == 1
                  ? 'Sharing in $secondsRemaining second...'
                  : 'Sharing in $secondsRemaining seconds...',
              style: theme.textTheme.titleMedium?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 8),
            Text(
              'This delay is intentional.',
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
            const SizedBox(height: 32),
            OutlinedButton(onPressed: onCancel, child: const Text('Cancel')),
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Countdown action — shown in the AppBar actions during countdown
// ---------------------------------------------------------------------------

class _CountdownAction extends StatelessWidget {
  const _CountdownAction({
    required this.secondsRemaining,
    required this.onCancel,
  });

  final int secondsRemaining;
  final VoidCallback onCancel;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Padding(
      padding: const EdgeInsets.only(right: 8),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 32,
            height: 32,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: theme.colorScheme.primaryContainer,
            ),
            child: Text(
              '$secondsRemaining',
              style: theme.textTheme.labelLarge?.copyWith(
                color: theme.colorScheme.onPrimaryContainer,
                fontWeight: FontWeight.bold,
              ),
            ),
          ),
          const SizedBox(width: 8),
          TextButton(onPressed: onCancel, child: const Text('Cancel')),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Quote notice
// ---------------------------------------------------------------------------

class _QuoteNotice extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      decoration: BoxDecoration(
        color: theme.colorScheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(8),
      ),
      child: Row(
        children: [
          Icon(Icons.info_outline, size: 16, color: theme.colorScheme.primary),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              'Quote posts require at least 5 distinct words.',
              style: theme.textTheme.labelMedium?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Character counter
// ---------------------------------------------------------------------------

class _CharacterCounter extends StatelessWidget {
  const _CharacterCounter({required this.runeCount, required this.maxRunes});

  final int runeCount;
  final int maxRunes;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final remaining = maxRunes - runeCount;
    final isNearLimit = remaining <= 50;
    final isOverLimit = remaining < 0;

    final color = isOverLimit
        ? theme.colorScheme.error
        : isNearLimit
        ? Colors.orange
        : theme.colorScheme.onSurfaceVariant;

    return Align(
      alignment: Alignment.centerRight,
      child: Padding(
        padding: const EdgeInsets.only(bottom: 8),
        child: Text(
          '$remaining',
          style: theme.textTheme.labelSmall?.copyWith(color: color),
        ),
      ),
    );
  }
}
