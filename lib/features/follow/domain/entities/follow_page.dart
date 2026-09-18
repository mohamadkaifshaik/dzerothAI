import 'package:equatable/equatable.dart';

import 'follow_user.dart';

/// A paginated page of [FollowUser] entries.
///
/// [terminated] is true when the backend has reached its configured boundary.
/// Per CLAUDE.md §2.1 the client must stop fetching when this is true.
class FollowPage extends Equatable {
  const FollowPage({
    required this.items,
    this.nextCursor,
    required this.terminated,
  });

  final List<FollowUser> items;

  /// Opaque cursor for the next page. Null when there are no further pages.
  final String? nextCursor;

  /// True when the server-enforced boundary has been reached.
  final bool terminated;

  @override
  List<Object?> get props => [items, nextCursor, terminated];
}
