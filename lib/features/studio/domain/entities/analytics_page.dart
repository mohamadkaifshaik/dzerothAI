import 'package:equatable/equatable.dart';

import 'post_analytics.dart';

/// A paginated page of [PostAnalytics] entries from the Creator Studio API.
///
/// [terminated] is true when the backend has reached the configured boundary.
/// Per CLAUDE.md §2.1 the client must stop fetching and show the termination
/// experience when this flag is true.
class AnalyticsPage extends Equatable {
  const AnalyticsPage({
    required this.items,
    this.nextCursor,
    required this.terminated,
  });

  final List<PostAnalytics> items;

  /// Opaque base64url cursor for the next page. Null when no further pages exist.
  final String? nextCursor;

  /// True when the server-enforced boundary has been reached.
  final bool terminated;

  @override
  List<Object?> get props => [items, nextCursor, terminated];
}
