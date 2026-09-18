part of 'auth_bloc.dart';

/// All states that [AuthBloc] can emit.
sealed class AuthState extends Equatable {
  const AuthState();

  @override
  List<Object?> get props => [];
}

/// The initial state before [AuthCheckRequested] has been handled.
final class AuthInitial extends AuthState {
  const AuthInitial();
}

/// A blocking auth operation (login, register, refresh on startup) is in
/// progress.
final class AuthLoading extends AuthState {
  const AuthLoading();
}

/// The user is authenticated. The access token is held here in memory only.
final class AuthAuthenticated extends AuthState {
  const AuthAuthenticated({required this.userId, required this.accessToken});

  final String userId;

  /// Short-lived JWT access token. Never persisted to disk.
  final String accessToken;

  @override
  List<Object?> get props => [userId, accessToken];
}

/// No valid session exists. The user must sign in.
final class AuthUnauthenticated extends AuthState {
  const AuthUnauthenticated();
}

/// An auth operation failed. Callers should inspect [failure] for the cause.
final class AuthError extends AuthState {
  const AuthError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
