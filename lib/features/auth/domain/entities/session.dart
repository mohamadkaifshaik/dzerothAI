import 'package:equatable/equatable.dart';

/// Represents an authenticated session.
///
/// The access token is short-lived (15 min) and held in memory only.
/// The refresh token is stored in flutter_secure_storage per ADR 0005.
class Session extends Equatable {
  const Session({
    required this.accessToken,
    required this.refreshToken,
    required this.userId,
  });

  /// Short-lived JWT. Held in BLoC state only — never written to disk.
  final String accessToken;

  /// Long-lived opaque token. Stored in flutter_secure_storage.
  final String refreshToken;

  /// The authenticated user's UUID v7.
  final String userId;

  @override
  List<Object?> get props => [accessToken, refreshToken, userId];
}
