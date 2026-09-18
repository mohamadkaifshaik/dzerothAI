import '../../domain/entities/profile.dart';

/// DTO for a public user profile (GET /api/v1/users/:id).
///
/// Maps from the API single-resource envelope:
/// ```json
/// { "data": { "id": "...", "handle": "...", ... } }
/// ```
///
/// IMPORTANT: The API contract forbids social-validation metrics in public
/// DTOs. This class does not declare any such field. If the backend ever
/// returns one, it is silently ignored here.
class ProfileDto {
  const ProfileDto({
    required this.id,
    required this.handle,
    required this.displayName,
    this.bio,
    this.avatarUrl,
    this.headerUrl,
    this.location,
    this.websiteUrl,
    required this.isPrivate,
    required this.joinedAt,
  });

  final String id;
  final String handle;
  final String displayName;
  final String? bio;
  final String? avatarUrl;
  final String? headerUrl;
  final String? location;
  final String? websiteUrl;
  final bool isPrivate;
  final String joinedAt;

  factory ProfileDto.fromJson(Map<String, dynamic> json) {
    // Unwrap the standard single-resource envelope if present.
    final data = json['data'] is Map<String, dynamic>
        ? json['data'] as Map<String, dynamic>
        : json;

    return ProfileDto(
      id: data['id'] as String,
      handle: data['handle'] as String,
      displayName: data['display_name'] as String,
      bio: data['bio'] as String?,
      avatarUrl: data['avatar_url'] as String?,
      headerUrl: data['header_url'] as String?,
      location: data['location'] as String?,
      websiteUrl: data['website_url'] as String?,
      isPrivate: (data['is_private'] as bool?) ?? false,
      joinedAt: data['joined_at'] as String,
    );
  }

  Profile toDomain() => Profile(
    id: id,
    handle: handle,
    displayName: displayName,
    bio: bio,
    avatarUrl: avatarUrl,
    headerUrl: headerUrl,
    location: location,
    websiteUrl: websiteUrl,
    isPrivate: isPrivate,
    joinedAt: joinedAt,
  );
}
