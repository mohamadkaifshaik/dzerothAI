import '../../../../core/error/result.dart';

/// Contract for account settings operations.
///
/// Provides access to the authenticated user's privacy settings
/// and account lifecycle operations.
abstract class SettingsRepository {
  /// Fetch the authenticated user's privacy settings.
  ///
  /// Maps to `GET /api/v1/me/settings`.
  Future<Result<SettingsData>> getSettings();

  /// Update the account privacy flag.
  ///
  /// Maps to `PUT /api/v1/me/settings`.
  Future<Result<SettingsData>> updatePrivacy({required bool isPrivate});

  /// Permanently suspend (deactivate) the authenticated user's account.
  ///
  /// Maps to `DELETE /api/v1/me/account`. Expects 204 No Content.
  Future<Result<void>> suspendAccount();
}

/// Domain model for account settings.
///
/// Intentionally minimal — settings screen only exposes what the user
/// can control. Internal server-side settings are not surfaced.
class SettingsData {
  const SettingsData({required this.isPrivate});

  final bool isPrivate;

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is SettingsData &&
          runtimeType == other.runtimeType &&
          isPrivate == other.isPrivate;

  @override
  int get hashCode => isPrivate.hashCode;
}
