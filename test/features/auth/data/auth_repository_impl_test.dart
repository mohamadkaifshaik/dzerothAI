import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/core/storage/secure_storage.dart';
import 'package:dzeroth/features/auth/data/repositories/auth_repository_impl.dart';

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

class MockDio extends Mock implements Dio {}

class MockSecureStorage extends Mock implements SecureStorage {}

// ---------------------------------------------------------------------------
// Test response data
// ---------------------------------------------------------------------------

// API response envelope matching AuthResponseDto.fromJson expectations.
// The dto checks for a 'data' key or falls back to the root map.
const _loginResponseData = {
  'data': {
    'access_token': 'eyJ.test.access',
    'refresh_token': 'raw-refresh-hex-token',
    'user_id': '01900000-0000-7000-8000-000000000002',
  },
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

Response<Map<String, dynamic>> _okResponse(Map<String, dynamic> data) {
  return Response<Map<String, dynamic>>(
    data: data,
    statusCode: 200,
    requestOptions: RequestOptions(path: '/'),
  );
}

DioException _dioError(int statusCode, Map<String, dynamic> body) {
  return DioException(
    requestOptions: RequestOptions(path: '/'),
    type: DioExceptionType.badResponse,
    response: Response(
      data: body,
      statusCode: statusCode,
      requestOptions: RequestOptions(path: '/'),
    ),
  );
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockDio mockDio;
  late MockSecureStorage mockStorage;
  late AuthRepositoryImpl repo;

  setUp(() {
    mockDio = MockDio();
    mockStorage = MockSecureStorage();
    repo = AuthRepositoryImpl(dio: mockDio, secureStorage: mockStorage);
  });

  setUpAll(() {
    registerFallbackValue(RequestOptions(path: '/'));
  });

  group('login', () {
    test(
        'stores refresh token in SecureStorage after successful login',
        () async {
      when(
        () => mockDio.post<Map<String, dynamic>>(
          any(),
          data: any(named: 'data'),
        ),
      ).thenAnswer((_) async => _okResponse(_loginResponseData));

      when(
        () => mockStorage.writeRefreshToken(any()),
      ).thenAnswer((_) async {});

      final result = await repo.login(
        email: 'user@example.com',
        password: 'password123',
      );

      expect(result, isA<Success<dynamic>>());
      verify(() => mockStorage.writeRefreshToken('raw-refresh-hex-token'))
          .called(1);
    });

    test('returns UnauthorizedFailure for 401 response', () async {
      when(
        () => mockDio.post<Map<String, dynamic>>(
          any(),
          data: any(named: 'data'),
        ),
      ).thenThrow(
        _dioError(401, {
          'error': {'code': 'UNAUTHORIZED', 'message': 'invalid credentials'},
        }),
      );

      final result = await repo.login(
        email: 'user@example.com',
        password: 'wrong',
      );

      expect(result, isA<Err<dynamic>>());
      final err = result as Err;
      expect(err.failure, isA<UnauthorizedFailure>());
    });
  });

  group('logout', () {
    test('clears refresh token from SecureStorage', () async {
      when(
        () => mockDio.post<void>(any()),
      ).thenAnswer((_) async => Response<void>(
            data: null,
            statusCode: 204,
            requestOptions: RequestOptions(path: '/'),
          ));

      when(
        () => mockStorage.deleteRefreshToken(),
      ).thenAnswer((_) async {});

      final result = await repo.logout();

      expect(result, isA<Success<dynamic>>());
      verify(() => mockStorage.deleteRefreshToken()).called(1);
    });

    test(
        'clears refresh token locally even when server call fails with NetworkFailure',
        () async {
      when(
        () => mockDio.post<void>(any()),
      ).thenThrow(
        DioException(
          requestOptions: RequestOptions(path: '/'),
          type: DioExceptionType.connectionError,
        ),
      );

      when(
        () => mockStorage.deleteRefreshToken(),
      ).thenAnswer((_) async {});

      final result = await repo.logout();

      // A NetworkFailure on logout still clears tokens and returns Success.
      expect(result, isA<Success<dynamic>>());
      verify(() => mockStorage.deleteRefreshToken()).called(1);
    });
  });

  group('refreshSession', () {
    test('reads refresh token from SecureStorage', () async {
      when(
        () => mockStorage.readRefreshToken(),
      ).thenAnswer((_) async => 'stored-refresh-token');

      when(
        () => mockDio.post<Map<String, dynamic>>(
          any(),
          data: any(named: 'data'),
        ),
      ).thenAnswer((_) async => _okResponse(_loginResponseData));

      when(
        () => mockStorage.writeRefreshToken(any()),
      ).thenAnswer((_) async {});

      final result = await repo.refreshSession();

      expect(result, isA<Success<dynamic>>());
      verify(() => mockStorage.readRefreshToken()).called(1);
    });

    test(
        'returns UnauthorizedFailure and does not call API when no stored token',
        () async {
      when(
        () => mockStorage.readRefreshToken(),
      ).thenAnswer((_) async => null);

      final result = await repo.refreshSession();

      expect(result, isA<Err<dynamic>>());
      final err = result as Err;
      expect(err.failure, isA<UnauthorizedFailure>());
      // API should never be called if there is no stored token.
      verifyNever(
        () => mockDio.post<Map<String, dynamic>>(
          any(),
          data: any(named: 'data'),
        ),
      );
    });

    test(
        'deletes stored refresh token and returns UnauthorizedFailure on 401',
        () async {
      when(
        () => mockStorage.readRefreshToken(),
      ).thenAnswer((_) async => 'expired-token');

      when(
        () => mockDio.post<Map<String, dynamic>>(
          any(),
          data: any(named: 'data'),
        ),
      ).thenThrow(
        _dioError(401, {
          'error': {
            'code': 'UNAUTHORIZED',
            'message': 'refresh token expired',
          },
        }),
      );

      when(
        () => mockStorage.deleteRefreshToken(),
      ).thenAnswer((_) async {});

      final result = await repo.refreshSession();

      expect(result, isA<Err<dynamic>>());
      final err = result as Err;
      expect(err.failure, isA<UnauthorizedFailure>());
      verify(() => mockStorage.deleteRefreshToken()).called(1);
    });
  });

  group('register', () {
    test('stores refresh token in SecureStorage after successful registration',
        () async {
      when(
        () => mockDio.post<Map<String, dynamic>>(
          any(),
          data: any(named: 'data'),
        ),
      ).thenAnswer((_) async => _okResponse(_loginResponseData));

      when(
        () => mockStorage.writeRefreshToken(any()),
      ).thenAnswer((_) async {});

      final result = await repo.register(
        handle: 'newuser',
        displayName: 'New User',
        email: 'new@example.com',
        password: 'StrongPass1!',
      );

      expect(result, isA<Success<dynamic>>());
      verify(() => mockStorage.writeRefreshToken('raw-refresh-hex-token'))
          .called(1);
    });

    test('returns ConflictFailure for 409 response (handle taken)', () async {
      when(
        () => mockDio.post<Map<String, dynamic>>(
          any(),
          data: any(named: 'data'),
        ),
      ).thenThrow(
        _dioError(409, {
          'error': {
            'code': 'CONFLICT',
            'message': 'That handle is already taken.',
          },
        }),
      );

      final result = await repo.register(
        handle: 'taken',
        displayName: 'User',
        email: 'taken@example.com',
        password: 'StrongPass1!',
      );

      expect(result, isA<Err<dynamic>>());
      final err = result as Err;
      expect(err.failure, isA<ConflictFailure>());
    });
  });
}
