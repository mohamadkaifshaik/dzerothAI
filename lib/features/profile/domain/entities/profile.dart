import 'package:equatable/equatable.dart';

/// Public profile of a Dzeroth user.
///
/// IMPORTANT: This entity must NOT include any social-validation metrics.
/// Fields forbidden per CLAUDE.md section 2.3 and API_CONTRACTS.md:
///   followerCount, followingCount, likeCount, postCount, impressionCount,
///   bookmarkCount, or any equivalent popularity metric.
///
/// These values are never rendered in public feed layers or profile screens.
class Profile extends Equatable {
  const Profile({
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

  /// UUID v7 identifier.
  final String id;

  /// Unique handle (without @).
  final String handle;

  /// Human-readable display name.
  final String displayName;

  final String? bio;
  final String? avatarUrl;
  final String? headerUrl;
  final String? location;
  final String? websiteUrl;

  /// Whether this profile's posts are visible to approved followers only.
  final bool isPrivate;

  /// ISO 8601 timestamp of account creation.
  final String joinedAt;

  @override
  List<Object?> get props => [
    id,
    handle,
    displayName,
    bio,
    avatarUrl,
    headerUrl,
    location,
    websiteUrl,
    isPrivate,
    joinedAt,
  ];
}
