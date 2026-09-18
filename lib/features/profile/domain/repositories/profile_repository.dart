import '../../../../core/error/result.dart';
import '../entities/own_profile.dart';
import '../entities/profile.dart';

/// Contract for profile data operations.
abstract class ProfileRepository {
  /// Get the authenticated user's own profile (GET /api/v1/me).
  Future<Result<OwnProfile>> getOwnProfile();

  /// Update the authenticated user's profile (PUT /api/v1/me).
  Future<Result<OwnProfile>> updateOwnProfile({
    String? displayName,
    String? bio,
    String? location,
    String? websiteUrl,
  });

  /// Get a user's public profile by UUID (GET /api/v1/users/:id).
  Future<Result<Profile>> getUserProfile(String userId);
}
