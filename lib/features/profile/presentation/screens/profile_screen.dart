import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:go_router/go_router.dart';

import '../../../../features/auth/presentation/bloc/auth_bloc.dart';
import '../../../../features/block/presentation/bloc/block_bloc.dart';
import '../../../../features/follow/presentation/bloc/follow_bloc.dart';
import '../../../../features/report/domain/repositories/report_repository.dart';
import '../../../../features/report/presentation/widgets/report_sheet.dart';
import '../../domain/entities/own_profile.dart';
import '../../domain/entities/profile.dart';
import '../bloc/profile_bloc.dart';

/// Displays a user's public profile.
///
/// PUBLIC METRICS LOCKDOWN: This screen must NOT display any social-validation
/// metrics (follower count, following count, like count, impression count, etc.)
/// per CLAUDE.md §2.3.
///
/// Accepts a [userId] and dispatches [ProfileLoadRequested] to load the profile.
/// When [isOwnProfile] is true it dispatches [OwnProfileLoadRequested] instead
/// and shows the edit button.
///
/// For other users' profiles:
///   - A follow/unfollow button is shown to authenticated viewers.
///   - A context menu (three-dot) provides mute/unmute and block/unblock actions.
///   - On [BlockSuccess] with action "blocked", the screen navigates away.
class ProfileScreen extends StatefulWidget {
  const ProfileScreen({
    super.key,
    required this.userId,
    this.isOwnProfile = false,
  });

  final String userId;
  final bool isOwnProfile;

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  @override
  void initState() {
    super.initState();
    _loadProfile();

    // For other users' profiles, check the initial follow status.
    if (!widget.isOwnProfile) {
      final authState = context.read<AuthBloc>().state;
      if (authState is AuthAuthenticated) {
        context.read<FollowBloc>().add(const FollowStatusCheckRequested());
      }
    }
  }

  void _loadProfile() {
    if (widget.isOwnProfile) {
      context.read<ProfileBloc>().add(const OwnProfileLoadRequested());
    } else {
      context.read<ProfileBloc>().add(
        ProfileLoadRequested(userId: widget.userId),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return MultiBlocListener(
      listeners: [
        // Navigate away when the user is blocked.
        BlocListener<BlockBloc, BlockState>(
          listener: (context, state) {
            if (state is BlockSuccess && state.action == 'blocked') {
              if (context.canPop()) {
                context.pop();
              } else {
                context.go('/home/feed');
              }
            }
          },
        ),
      ],
      child: Scaffold(
        body: BlocBuilder<ProfileBloc, ProfileState>(
          builder: (context, state) {
            return switch (state) {
              ProfileLoading() || ProfileInitial() => _buildLoading(),
              ProfileLoaded(:final profile) => _buildProfile(context, profile),
              ProfileError(:final failure) => _buildError(
                context,
                failure.message,
              ),
              _ => _buildLoading(),
            };
          },
        ),
      ),
    );
  }

  Widget _buildLoading() {
    return const CustomScrollView(
      slivers: [
        SliverAppBar(expandedHeight: 150, flexibleSpace: FlexibleSpaceBar()),
        SliverFillRemaining(child: Center(child: CircularProgressIndicator())),
      ],
    );
  }

  Widget _buildError(BuildContext context, String message) {
    return CustomScrollView(
      slivers: [
        const SliverAppBar(expandedHeight: 150),
        SliverFillRemaining(
          child: Center(
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
                    onPressed: _loadProfile,
                    icon: const Icon(Icons.refresh),
                    label: const Text('Try again'),
                  ),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildProfile(BuildContext context, Profile profile) {
    final theme = Theme.of(context);
    final isOwn = profile is OwnProfile;

    // Determine if the viewer is authenticated and viewing another user.
    final authState = context.watch<AuthBloc>().state;
    final viewerIsAuthenticated = authState is AuthAuthenticated;
    final showSocialActions = !isOwn && viewerIsAuthenticated;

    return CustomScrollView(
      slivers: [
        SliverAppBar(
          expandedHeight: 150,
          pinned: true,
          actions: [
            if (isOwn)
              IconButton(
                icon: const Icon(Icons.edit_outlined),
                tooltip: 'Edit profile',
                onPressed: () => context.push('/users/${profile.id}/edit'),
              ),
            if (showSocialActions)
              _BlockMuteMenuButton(targetUserId: widget.userId),
          ],
          flexibleSpace: FlexibleSpaceBar(
            background: _HeaderArea(headerUrl: profile.headerUrl),
          ),
        ),
        SliverToBoxAdapter(
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const SizedBox(height: 12),

                // Avatar row with action buttons.
                Row(
                  crossAxisAlignment: CrossAxisAlignment.end,
                  children: [
                    _AvatarWidget(
                      avatarUrl: profile.avatarUrl,
                      displayName: profile.displayName,
                    ),
                    const Spacer(),
                    // Follow/unfollow button for other users' profiles.
                    if (showSocialActions) const _FollowButton(),
                  ],
                ),
                const SizedBox(height: 12),

                // Display name
                Text(profile.displayName, style: theme.textTheme.titleLarge),
                const SizedBox(height: 2),

                // Handle
                Text(
                  '@${profile.handle}',
                  style: theme.textTheme.bodyMedium?.copyWith(
                    color: theme.colorScheme.onSurfaceVariant,
                  ),
                ),

                // Bio
                if (profile.bio != null && profile.bio!.isNotEmpty) ...[
                  const SizedBox(height: 12),
                  Text(profile.bio!, style: theme.textTheme.bodyMedium),
                ],

                const SizedBox(height: 12),

                // Location and website
                Wrap(
                  spacing: 16,
                  runSpacing: 4,
                  children: [
                    if (profile.location != null &&
                        profile.location!.isNotEmpty)
                      _MetaChip(
                        icon: Icons.location_on_outlined,
                        label: profile.location!,
                      ),
                    if (profile.websiteUrl != null &&
                        profile.websiteUrl!.isNotEmpty)
                      _MetaChip(icon: Icons.link, label: profile.websiteUrl!),
                    _MetaChip(
                      icon: Icons.calendar_today_outlined,
                      label: 'Joined ${_formatJoinedAt(profile.joinedAt)}',
                    ),
                  ],
                ),

                const SizedBox(height: 16),

                // Creator Studio — only shown to the authenticated owner.
                // PRIVATE: This entry point must NOT appear on other users'
                // profiles per CLAUDE.md §2.3.
                if (isOwn) ...[
                  OutlinedButton.icon(
                    onPressed: () => context.push('/studio'),
                    icon: const Icon(Icons.bar_chart_outlined),
                    label: const Text('Creator Studio'),
                  ),
                  const SizedBox(height: 12),
                ],

                const Divider(),

                // Placeholder for future posts timeline
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 32),
                  child: Center(
                    child: Text(
                      'Posts coming in Phase 2.',
                      style: theme.textTheme.bodyMedium?.copyWith(
                        color: theme.colorScheme.onSurfaceVariant,
                      ),
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }

  String _formatJoinedAt(String iso) {
    try {
      final dt = DateTime.parse(iso);
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
      return '${months[dt.month - 1]} ${dt.year}';
    } catch (_) {
      return iso;
    }
  }
}

// ---------------------------------------------------------------------------
// Follow button
// ---------------------------------------------------------------------------

/// Follow/unfollow toggle button.
///
/// Reads [FollowBloc] from context — the BLoC must be provided above this
/// widget in the tree (at the profile screen level).
class _FollowButton extends StatelessWidget {
  const _FollowButton();

  @override
  Widget build(BuildContext context) {
    return BlocBuilder<FollowBloc, FollowState>(
      builder: (context, state) {
        // Show a spinner while loading.
        if (state is FollowLoading) {
          return const SizedBox(
            width: 88,
            height: 36,
            child: Center(
              child: SizedBox(
                width: 20,
                height: 20,
                child: CircularProgressIndicator(strokeWidth: 2),
              ),
            ),
          );
        }

        final isFollowing = state is FollowSuccess && state.isFollowing;

        return isFollowing
            ? OutlinedButton(
                onPressed: () =>
                    context.read<FollowBloc>().add(const UnfollowRequested()),
                child: const Text('Following'),
              )
            : FilledButton(
                onPressed: () =>
                    context.read<FollowBloc>().add(const FollowRequested()),
                child: const Text('Follow'),
              );
      },
    );
  }
}

// ---------------------------------------------------------------------------
// Block / mute context menu
// ---------------------------------------------------------------------------

/// Three-dot context menu for block and mute actions on another user's profile.
///
/// Reads [BlockBloc] from context — the BLoC must be provided above this
/// widget in the tree (at the profile screen level).
class _BlockMuteMenuButton extends StatelessWidget {
  const _BlockMuteMenuButton({required this.targetUserId});

  final String targetUserId;

  @override
  Widget build(BuildContext context) {
    return BlocBuilder<BlockBloc, BlockState>(
      builder: (context, state) {
        return PopupMenuButton<_ProfileAction>(
          icon: const Icon(Icons.more_vert),
          tooltip: 'More options',
          onSelected: (action) => _handleAction(context, action),
          itemBuilder: (_) => const [
            PopupMenuItem(
              value: _ProfileAction.mute,
              child: ListTile(
                leading: Icon(Icons.volume_off_outlined),
                title: Text('Mute'),
                contentPadding: EdgeInsets.zero,
              ),
            ),
            PopupMenuItem(
              value: _ProfileAction.block,
              child: ListTile(
                leading: Icon(Icons.block_outlined),
                title: Text('Block'),
                contentPadding: EdgeInsets.zero,
              ),
            ),
            PopupMenuItem(
              value: _ProfileAction.report,
              child: ListTile(
                leading: Icon(Icons.flag_outlined),
                title: Text('Report user'),
                contentPadding: EdgeInsets.zero,
              ),
            ),
          ],
        );
      },
    );
  }

  void _handleAction(BuildContext context, _ProfileAction action) {
    switch (action) {
      case _ProfileAction.mute:
        context.read<BlockBloc>().add(const MuteUserRequested());
      case _ProfileAction.block:
        context.read<BlockBloc>().add(const BlockUserRequested());
      case _ProfileAction.report:
        final repo = _tryReadReportRepo(context);
        if (repo == null) return;
        showModalBottomSheet<void>(
          context: context,
          isScrollControlled: true,
          builder: (_) =>
              ReportSheet.forUser(userId: targetUserId, reportRepository: repo),
        );
    }
  }

  ReportRepository? _tryReadReportRepo(BuildContext context) {
    try {
      return context.read<ReportRepository>();
    } catch (_) {
      return null;
    }
  }
}

enum _ProfileAction { mute, block, report }

// ---------------------------------------------------------------------------
// Shared private widgets (unchanged from original)
// ---------------------------------------------------------------------------

class _HeaderArea extends StatelessWidget {
  const _HeaderArea({this.headerUrl});

  final String? headerUrl;

  @override
  Widget build(BuildContext context) {
    if (headerUrl != null && headerUrl!.isNotEmpty) {
      return Image.network(
        headerUrl!,
        fit: BoxFit.cover,
        errorBuilder: (context2, err, stackTrace) => _placeholder(context),
      );
    }
    return _placeholder(context);
  }

  Widget _placeholder(BuildContext context) {
    return Container(
      color: Theme.of(context).colorScheme.surfaceContainerHighest,
    );
  }
}

class _AvatarWidget extends StatelessWidget {
  const _AvatarWidget({required this.avatarUrl, required this.displayName});

  final String? avatarUrl;
  final String displayName;

  @override
  Widget build(BuildContext context) {
    final initials = displayName.isNotEmpty
        ? displayName.trim()[0].toUpperCase()
        : '?';

    return CircleAvatar(
      radius: 40,
      backgroundColor: Theme.of(context).colorScheme.primaryContainer,
      backgroundImage: (avatarUrl != null && avatarUrl!.isNotEmpty)
          ? NetworkImage(avatarUrl!)
          : null,
      child: (avatarUrl == null || avatarUrl!.isEmpty)
          ? Text(
              initials,
              style: const TextStyle(fontSize: 28, fontWeight: FontWeight.w600),
            )
          : null,
    );
  }
}

class _MetaChip extends StatelessWidget {
  const _MetaChip({required this.icon, required this.label});

  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(
          icon,
          size: 16,
          color: Theme.of(context).colorScheme.onSurfaceVariant,
        ),
        const SizedBox(width: 4),
        Text(
          label,
          style: Theme.of(context).textTheme.bodySmall
              ?.copyWith(color: Theme.of(context).colorScheme.onSurfaceVariant),
        ),
      ],
    );
  }
}
