import 'package:equatable/equatable.dart';

/// A compact title summary embedded in author/profile payloads.
///
/// Only [slug] and [displayName] are exposed publicly.
/// Per CLAUDE.md §2.3, no social-validation metrics are included here.
class TitleSummary extends Equatable {
  const TitleSummary({required this.slug, required this.displayName});

  final String slug;
  final String displayName;

  @override
  List<Object?> get props => [slug, displayName];
}

/// A title owned by the authenticated user, returned from GET /titles/me.
///
/// [isRevocable], [status], and [category] are private fields that provide
/// management context for the Title Library screen. They must NOT be
/// exposed in any public feed or profile surface.
class UserTitle extends Equatable {
  const UserTitle({
    required this.id,
    required this.slug,
    required this.displayName,
    required this.category,
    required this.isRevocable,
    required this.status,
    required this.unlockedAt,
  });

  final String id;
  final String slug;
  final String displayName;
  final String category;
  final bool isRevocable;

  /// Backend ENUM: 'active', 'grace_period', etc.
  final String status;

  final DateTime unlockedAt;

  @override
  List<Object?> get props => [
    id,
    slug,
    displayName,
    category,
    isRevocable,
    status,
    unlockedAt,
  ];
}
