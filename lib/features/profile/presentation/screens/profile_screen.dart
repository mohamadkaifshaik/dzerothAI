import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:go_router/go_router.dart';

import '../../domain/entities/own_profile.dart';
import '../../domain/entities/profile.dart';
import '../bloc/profile_bloc.dart';

/// Displays a user's public profile.
///
/// PUBLIC METRICS LOCKDOWN: This screen must NOT display any social-validation
/// metrics (follower count, following count, like count, impression count, etc.)
/// per CLAUDE.md section 2.3.
///
/// Accepts a [userId] and dispatches [ProfileLoadRequested] to load the profile.
/// When [isOwnProfile] is true it dispatches [OwnProfileLoadRequested] instead
/// and shows the edit button.
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
    return Scaffold(
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

                // Avatar row with edit button for own profile
                Row(
                  crossAxisAlignment: CrossAxisAlignment.end,
                  children: [
                    _AvatarWidget(
                      avatarUrl: profile.avatarUrl,
                      displayName: profile.displayName,
                    ),
                    const Spacer(),
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
