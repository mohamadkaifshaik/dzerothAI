// ignore_for_file: prefer_initializing_formals
import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/repositories/auth_repository.dart';

part 'auth_event.dart';
part 'auth_state.dart';

/// Manages authentication lifecycle for the Dzeroth app.
///
/// Per ADR 0005:
/// - Access token is held in [AuthAuthenticated.accessToken] (memory only).
/// - Refresh token is persisted by [AuthRepository] via SecureStorage.
/// - On app start, [AuthCheckRequested] attempts a silent refresh.
/// - [AuthInterceptor] dispatches [AuthTokenRefreshed] / [AuthSessionExpired]
///   to keep this BLoC's access token current without a full sign-in.
class AuthBloc extends Bloc<AuthEvent, AuthState> {
  AuthBloc({required AuthRepository authRepository})
    : _authRepository = authRepository,
      super(const AuthInitial()) {
    on<AuthCheckRequested>(_onCheckRequested);
    on<AuthLoginRequested>(_onLoginRequested);
    on<AuthRegisterRequested>(_onRegisterRequested);
    on<AuthLogoutRequested>(_onLogoutRequested);
    on<AuthTokenRefreshed>(_onTokenRefreshed);
    on<AuthSessionExpired>(_onSessionExpired);
  }

  final AuthRepository _authRepository;

  Future<void> _onCheckRequested(
    AuthCheckRequested event,
    Emitter<AuthState> emit,
  ) async {
    emit(const AuthLoading());
    final result = await _authRepository.refreshSession();
    switch (result) {
      case Success(:final value):
        emit(
          AuthAuthenticated(
            userId: value.userId,
            accessToken: value.accessToken,
          ),
        );
      case Err():
        // An UnauthorizedFailure here is expected (no stored token / expired).
        // Any other failure type is treated the same way: show login.
        // We do not surface this error to the user on cold start.
        emit(const AuthUnauthenticated());
    }
  }

  Future<void> _onLoginRequested(
    AuthLoginRequested event,
    Emitter<AuthState> emit,
  ) async {
    emit(const AuthLoading());
    final result = await _authRepository.login(
      email: event.email,
      password: event.password,
    );
    switch (result) {
      case Success(:final value):
        emit(
          AuthAuthenticated(
            userId: value.userId,
            accessToken: value.accessToken,
          ),
        );
      case Err(:final failure):
        emit(AuthError(failure: failure));
    }
  }

  Future<void> _onRegisterRequested(
    AuthRegisterRequested event,
    Emitter<AuthState> emit,
  ) async {
    emit(const AuthLoading());
    final result = await _authRepository.register(
      handle: event.handle,
      displayName: event.displayName,
      email: event.email,
      password: event.password,
    );
    switch (result) {
      case Success(:final value):
        emit(
          AuthAuthenticated(
            userId: value.userId,
            accessToken: value.accessToken,
          ),
        );
      case Err(:final failure):
        emit(AuthError(failure: failure));
    }
  }

  Future<void> _onLogoutRequested(
    AuthLogoutRequested event,
    Emitter<AuthState> emit,
  ) async {
    emit(const AuthLoading());
    await _authRepository.logout();
    emit(const AuthUnauthenticated());
  }

  void _onTokenRefreshed(AuthTokenRefreshed event, Emitter<AuthState> emit) {
    final current = state;
    if (current is AuthAuthenticated) {
      emit(
        AuthAuthenticated(
          userId: current.userId,
          accessToken: event.accessToken,
        ),
      );
    }
  }

  void _onSessionExpired(AuthSessionExpired event, Emitter<AuthState> emit) {
    emit(const AuthUnauthenticated());
  }
}
