import 'package:equatable/equatable.dart';

/// Minimal user record returned in follow/follower lists.
///
/// PUBLIC METRICS LOCKDOWN: This entity intentionally carries no
/// follower count, following count, or any other social-validation metric
/// per CLAUDE.md §2.3.
class FollowUser extends Equatable {
  const FollowUser({
    required this.id,
    required this.handle,
    required this.displayName,
    this.avatarUrl,
  });

  final String id;
  final String handle;
  final String displayName;
  final String? avatarUrl;

  @override
  List<Object?> get props => [id, handle, displayName, avatarUrl];
}
