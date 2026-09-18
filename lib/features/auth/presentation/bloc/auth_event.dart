part of 'auth_bloc.dart';

/// All events that can be dispatched to [AuthBloc].
sealed class AuthEvent {
  const AuthEvent();
}

/// Dispatched on app start to attempt restoring a session from the stored
/// refresh token (ADR 0005: access token is not persisted).
final class AuthCheckRequested extends AuthEvent {
  const AuthCheckRequested();
}

/// Dispatched when the user submits the login form.
final class AuthLoginRequested extends AuthEvent {
  const AuthLoginRequested({required this.email, required this.password});

  final String email;
  final String password;
}

/// Dispatched when the user submits the registration form.
final class AuthRegisterRequested extends AuthEvent {
  const AuthRegisterRequested({
    required this.handle,
    required this.displayName,
    required this.email,
    required this.password,
  });

  final String handle;
  final String displayName;
  final String email;
  final String password;
}

/// Dispatched when the user requests sign-out.
final class AuthLogoutRequested extends AuthEvent {
  const AuthLogoutRequested();
}

/// Dispatched by [AuthInterceptor] when a silent token refresh succeeds.
/// Updates the in-memory access token without changing the authenticated state.
final class AuthTokenRefreshed extends AuthEvent {
  const AuthTokenRefreshed({
    required this.accessToken,
    required this.refreshToken,
  });

  final String accessToken;
  final String refreshToken;
}

/// Dispatched by [AuthInterceptor] when a silent refresh fails and the
/// session can no longer be recovered.
final class AuthSessionExpired extends AuthEvent {
  const AuthSessionExpired();
}
