import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../../../core/storage/secure_storage.dart';
import '../../domain/entities/session.dart';
import '../../domain/repositories/auth_repository.dart';
import '../models/auth_dto.dart';

/// Concrete implementation of [AuthRepository] backed by the Dzeroth API.
class AuthRepositoryImpl implements AuthRepository {
  AuthRepositoryImpl({required this.dio, required this.secureStorage});

  final Dio dio;
  final SecureStorage secureStorage;

  @override
  Future<Result<Session>> register({
    required String handle,
    required String displayName,
    required String email,
    required String password,
  }) async {
    try {
      final response = await dio.post<Map<String, dynamic>>(
        '/api/v1/auth/register',
        data: {
          'handle': handle,
          'display_name': displayName,
          'email': email,
          'password': password,
        },
      );

      final dto = AuthResponseDto.fromJson(response.data!);
      await secureStorage.writeRefreshToken(dto.tokens.refreshToken);
      return Success(
        Session(
          accessToken: dto.tokens.accessToken,
          refreshToken: dto.tokens.refreshToken,
          userId: dto.userId,
        ),
      );
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<Session>> login({
    required String email,
    required String password,
  }) async {
    try {
      final response = await dio.post<Map<String, dynamic>>(
        '/api/v1/auth/login',
        data: {'email': email, 'password': password},
      );

      final dto = AuthResponseDto.fromJson(response.data!);
      await secureStorage.writeRefreshToken(dto.tokens.refreshToken);
      return Success(
        Session(
          accessToken: dto.tokens.accessToken,
          refreshToken: dto.tokens.refreshToken,
          userId: dto.userId,
        ),
      );
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<Session>> refreshSession() async {
    final refreshToken = await secureStorage.readRefreshToken();
    if (refreshToken == null) {
      return const Err(UnauthorizedFailure('No stored session.'));
    }

    try {
      final response = await dio.post<Map<String, dynamic>>(
        '/api/v1/auth/refresh',
        data: {'refresh_token': refreshToken},
      );

      final dto = AuthResponseDto.fromJson(response.data!);
      await secureStorage.writeRefreshToken(dto.tokens.refreshToken);
      return Success(
        Session(
          accessToken: dto.tokens.accessToken,
          refreshToken: dto.tokens.refreshToken,
          userId: dto.userId,
        ),
      );
    } on DioException catch (e) {
      final failure = mapDioError(e);
      if (failure is UnauthorizedFailure) {
        await secureStorage.deleteRefreshToken();
      }
      return Err(failure);
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<void>> logout() async {
    try {
      await dio.post<void>('/api/v1/auth/logout');
    } on DioException catch (e) {
      // Even if the server call fails, clear local credentials so the user
      // is locally signed out. Only propagate non-network errors.
      final failure = mapDioError(e);
      if (failure is! NetworkFailure) {
        await secureStorage.deleteRefreshToken();
        return Err(failure);
      }
    } catch (_) {
      // Best-effort server call; always clear locally.
    }
    await secureStorage.deleteRefreshToken();
    return const Success(null);
  }
}
