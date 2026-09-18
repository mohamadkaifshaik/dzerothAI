import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/auth/domain/entities/session.dart';
import 'package:dzeroth/features/auth/domain/repositories/auth_repository.dart';
import 'package:dzeroth/features/auth/presentation/bloc/auth_bloc.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockAuthRepository extends Mock implements AuthRepository {}

// ---------------------------------------------------------------------------
// Fixture data
// ---------------------------------------------------------------------------

const _testSession = Session(
  accessToken: 'test.access.token',
  refreshToken: 'test-refresh-token',
  userId: '01900000-0000-7000-8000-000000000001',
);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockAuthRepository mockRepo;

  setUp(() {
    mockRepo = MockAuthRepository();
  });

  // Register fallback values for named parameters used in when() stubs.
  setUpAll(() {
    registerFallbackValue(
      const Session(accessToken: '', refreshToken: '', userId: ''),
    );
  });

  group('AuthCheckRequested', () {
    blocTest<AuthBloc, AuthState>(
      'emits [AuthLoading, AuthUnauthenticated] when no stored refresh token',
      build: () {
        when(() => mockRepo.refreshSession()).thenAnswer(
          (_) async => const Err(UnauthorizedFailure('No stored session.')),
        );
        return AuthBloc(authRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const AuthCheckRequested()),
      expect: () => [const AuthLoading(), const AuthUnauthenticated()],
    );

    blocTest<AuthBloc, AuthState>(
      'emits [AuthLoading, AuthAuthenticated] when stored refresh token is valid',
      build: () {
        when(() => mockRepo.refreshSession())
            .thenAnswer((_) async => const Success(_testSession));
        return AuthBloc(authRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const AuthCheckRequested()),
      expect: () => [
        const AuthLoading(),
        AuthAuthenticated(
          userId: _testSession.userId,
          accessToken: _testSession.accessToken,
        ),
      ],
    );

    blocTest<AuthBloc, AuthState>(
      'emits [AuthLoading, AuthUnauthenticated] when stored refresh token is invalid',
      build: () {
        when(() => mockRepo.refreshSession()).thenAnswer(
          (_) async => const Err(UnauthorizedFailure('Token expired.')),
        );
        return AuthBloc(authRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const AuthCheckRequested()),
      expect: () => [const AuthLoading(), const AuthUnauthenticated()],
    );
  });

  group('AuthLoginRequested', () {
    blocTest<AuthBloc, AuthState>(
      'emits [AuthLoading, AuthAuthenticated] with valid credentials',
      build: () {
        when(
          () => mockRepo.login(
            email: any(named: 'email'),
            password: any(named: 'password'),
          ),
        ).thenAnswer((_) async => const Success(_testSession));
        return AuthBloc(authRepository: mockRepo);
      },
      act: (bloc) => bloc.add(
        const AuthLoginRequested(
          email: 'user@example.com',
          password: 'correct-password',
        ),
      ),
      expect: () => [
        const AuthLoading(),
        AuthAuthenticated(
          userId: _testSession.userId,
          accessToken: _testSession.accessToken,
        ),
      ],
    );

    blocTest<AuthBloc, AuthState>(
      'emits [AuthLoading, AuthError(UnauthorizedFailure)] with invalid credentials',
      build: () {
        when(
          () => mockRepo.login(
            email: any(named: 'email'),
            password: any(named: 'password'),
          ),
        ).thenAnswer(
          (_) async => const Err(UnauthorizedFailure('invalid credentials')),
        );
        return AuthBloc(authRepository: mockRepo);
      },
      act: (bloc) => bloc.add(
        const AuthLoginRequested(
          email: 'user@example.com',
          password: 'wrong-password',
        ),
      ),
      expect: () => [
        const AuthLoading(),
        isA<AuthError>().having(
          (s) => s.failure,
          'failure',
          isA<UnauthorizedFailure>(),
        ),
      ],
    );
  });

  group('AuthLogoutRequested', () {
    blocTest<AuthBloc, AuthState>(
      'emits [AuthLoading, AuthUnauthenticated]',
      build: () {
        when(() => mockRepo.logout())
            .thenAnswer((_) async => const Success(null));
        return AuthBloc(authRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const AuthLogoutRequested()),
      expect: () => [const AuthLoading(), const AuthUnauthenticated()],
    );
  });

  group('AuthRegisterRequested', () {
    blocTest<AuthBloc, AuthState>(
      'emits [AuthLoading, AuthAuthenticated] on successful registration',
      build: () {
        when(
          () => mockRepo.register(
            handle: any(named: 'handle'),
            displayName: any(named: 'displayName'),
            email: any(named: 'email'),
            password: any(named: 'password'),
          ),
        ).thenAnswer((_) async => const Success(_testSession));
        return AuthBloc(authRepository: mockRepo);
      },
      act: (bloc) => bloc.add(
        const AuthRegisterRequested(
          handle: 'newuser',
          displayName: 'New User',
          email: 'new@example.com',
          password: 'StrongPass1!',
        ),
      ),
      expect: () => [
        const AuthLoading(),
        AuthAuthenticated(
          userId: _testSession.userId,
          accessToken: _testSession.accessToken,
        ),
      ],
    );

    blocTest<AuthBloc, AuthState>(
      'emits [AuthLoading, AuthError(ConflictFailure)] when handle is already taken',
      build: () {
        when(
          () => mockRepo.register(
            handle: any(named: 'handle'),
            displayName: any(named: 'displayName'),
            email: any(named: 'email'),
            password: any(named: 'password'),
          ),
        ).thenAnswer(
          (_) async =>
              const Err(ConflictFailure('That handle is already taken.')),
        );
        return AuthBloc(authRepository: mockRepo);
      },
      act: (bloc) => bloc.add(
        const AuthRegisterRequested(
          handle: 'taken',
          displayName: 'Existing User',
          email: 'taken@example.com',
          password: 'StrongPass1!',
        ),
      ),
      expect: () => [
        const AuthLoading(),
        isA<AuthError>().having(
          (s) => s.failure,
          'failure',
          isA<ConflictFailure>(),
        ),
      ],
    );
  });
}
