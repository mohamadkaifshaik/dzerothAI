import '../../domain/entities/own_profile.dart';

/// DTO for the authenticated user's own profile (GET /api/v1/me, PUT /api/v1/me).
///
/// Extends the public profile shape with private fields (email, email_verified).
/// These private fields must never be passed to a widget that renders a public
/// profile view.
class OwnProfileDto {
  const OwnProfileDto({
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
    required this.email,
    required this.emailVerified,
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
  final String email;
  final bool emailVerified;

  factory OwnProfileDto.fromJson(Map<String, dynamic> json) {
    final data = json['data'] is Map<String, dynamic>
        ? json['data'] as Map<String, dynamic>
        : json;

    return OwnProfileDto(
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
      email: data['email'] as String,
      emailVerified: (data['email_verified'] as bool?) ?? false,
    );
  }

  OwnProfile toDomain() => OwnProfile(
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
    email: email,
    emailVerified: emailVerified,
  );
}
