import 'package:equatable/equatable.dart';

import 'bookmark_item.dart';

/// A paginated page of [BookmarkItem] entries.
///
/// [terminated] is true when the backend has reached the configured boundary
/// (200 items). Per CLAUDE.md §2.1 the client must stop fetching and show the
/// termination experience when this is true.
class BookmarkPage extends Equatable {
  const BookmarkPage({
    required this.items,
    this.nextCursor,
    required this.terminated,
  });

  final List<BookmarkItem> items;

  /// Opaque cursor for the next page. Null when no further pages exist.
  final String? nextCursor;

  /// True when the server-enforced boundary has been reached.
  final bool terminated;

  @override
  List<Object?> get props => [items, nextCursor, terminated];
}
