import '../../domain/entities/user_title.dart';

/// DTO for a title summary embedded in author/profile payloads.
///
/// Matches the Go `PrimaryTitleDTO` JSON shape:
///   { "slug": "...", "display_name": "..." }
class TitleSummaryDto {
  const TitleSummaryDto({required this.slug, required this.displayName});

  final String slug;
  final String displayName;

  factory TitleSummaryDto.fromJson(Map<String, dynamic> json) {
    return TitleSummaryDto(
      slug: json['slug'] as String,
      displayName: json['display_name'] as String,
    );
  }

  TitleSummary toEntity() => TitleSummary(slug: slug, displayName: displayName);
}

/// DTO for a single user-owned title returned by GET /titles/me.
///
/// Matches the Go `UserTitleDTO` JSON shape.
class UserTitleDto {
  const UserTitleDto({
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
  final String status;
  final String unlockedAt;

  factory UserTitleDto.fromJson(Map<String, dynamic> json) {
    return UserTitleDto(
      id: json['id'] as String,
      slug: json['slug'] as String,
      displayName: json['display_name'] as String,
      category: (json['category'] as String?) ?? '',
      isRevocable: (json['is_revocable'] as bool?) ?? false,
      status: (json['status'] as String?) ?? '',
      unlockedAt: (json['unlocked_at'] as String?) ?? '',
    );
  }

  UserTitle toEntity() => UserTitle(
    id: id,
    slug: slug,
    displayName: displayName,
    category: category,
    isRevocable: isRevocable,
    status: status,
    unlockedAt: _parseDate(unlockedAt),
  );

  static DateTime _parseDate(String iso) {
    try {
      return DateTime.parse(iso);
    } catch (_) {
      return DateTime.fromMillisecondsSinceEpoch(0);
    }
  }
}
