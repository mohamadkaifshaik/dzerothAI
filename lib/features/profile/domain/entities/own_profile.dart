import 'profile.dart';

/// The authenticated user's own profile.
///
/// Extends [Profile] with private fields that are only available via
/// GET /api/v1/me (requires auth). These fields must never appear in
/// public profile displays.
class OwnProfile extends Profile {
  const OwnProfile({
    required super.id,
    required super.handle,
    required super.displayName,
    super.bio,
    super.avatarUrl,
    super.headerUrl,
    super.location,
    super.websiteUrl,
    required super.isPrivate,
    required super.joinedAt,
    required this.email,
    required this.emailVerified,
  });

  /// The user's private email address. Never displayed publicly.
  final String email;

  /// Whether the email address has been verified.
  final bool emailVerified;

  @override
  List<Object?> get props => [...super.props, email, emailVerified];
}
