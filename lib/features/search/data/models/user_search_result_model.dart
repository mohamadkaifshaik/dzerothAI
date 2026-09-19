import '../../domain/entities/user_search_result.dart';

/// DTO for a single user result from the search API.
///
/// Field names match the Go JSON tags returned by
/// `GET /api/v1/search/users?q=...`.
///
/// PUBLIC METRICS LOCKDOWN: This model intentionally contains zero
/// social-validation metric fields per CLAUDE.md §2.3.
/// Do not add follower_count, following_count, impressions, or any
/// equivalent field.
class UserSearchResultModel {
  const UserSearchResultModel({
    required this.id,
    required this.handle,
    required this.displayName,
    this.avatarUrl,
  });

  final String id;
  final String handle;
  final String displayName;
  final String? avatarUrl;

  factory UserSearchResultModel.fromJson(Map<String, dynamic> json) {
    return UserSearchResultModel(
      id: json['id'] as String,
      handle: json['handle'] as String,
      displayName: json['display_name'] as String,
      avatarUrl: json['avatar_url'] as String?,
    );
  }

  UserSearchResult toEntity() => UserSearchResult(
    id: id,
    handle: handle,
    displayName: displayName,
    avatarUrl: avatarUrl,
  );
}
