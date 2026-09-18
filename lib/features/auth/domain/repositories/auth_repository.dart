import '../../../../core/error/result.dart';
import '../entities/session.dart';

/// Contract for authentication operations.
///
/// All methods return [Result] so callers handle both success and failure
/// explicitly without relying on exception propagation.
abstract class AuthRepository {
  /// Create a new account and return the initial session.
  Future<Result<Session>> register({
    required String handle,
    required String displayName,
    required String email,
    required String password,
  });

  /// Authenticate with email + password and return a session.
  Future<Result<Session>> login({
    required String email,
    required String password,
  });

  /// Exchange the stored refresh token for a new token pair.
  ///
  /// Used on app start and by [AuthInterceptor] for silent refresh.
  Future<Result<Session>> refreshSession();

  /// Revoke the current session and clear local credentials.
  Future<Result<void>> logout();
}
