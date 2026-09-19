import 'package:equatable/equatable.dart';

/// A user result returned by the search API.
///
/// PUBLIC METRICS LOCKDOWN: This entity intentionally carries zero
/// social-validation metric fields (follower count, following count,
/// impressions, or any equivalent popularity metric) per CLAUDE.md §2.3.
class UserSearchResult extends Equatable {
  const UserSearchResult({
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
