import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/entities/follow_page.dart';
import '../../domain/entities/follow_user.dart';
import '../../domain/repositories/follow_repository.dart';

/// Concrete implementation of [FollowRepository] backed by the Dzeroth API.
///
/// Uses the shared authenticated [Dio] instance provided via constructor
/// injection. The [AuthInterceptor] handles token attachment transparently.
///
/// Endpoints:
///   POST   /api/v1/users/{userId}/follow        → 204
///   DELETE /api/v1/users/{userId}/follow        → 204
///   GET    /api/v1/users/{userId}/following     → { items, next_cursor, terminated }
///   GET    /api/v1/users/{userId}/followers     → { items, next_cursor, terminated }
class FollowRepositoryImpl implements FollowRepository {
  const FollowRepositoryImpl({required this.dio});

  final Dio dio;

  @override
  Future<Result<void>> follow(String targetUserId) async {
    try {
      await dio.post<void>('/api/v1/users/$targetUserId/follow');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<void>> unfollow(String targetUserId) async {
    try {
      await dio.delete<void>('/api/v1/users/$targetUserId/follow');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<bool>> isFollowing(String targetUserId) async {
    try {
      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/users/$targetUserId/follow',
      );
      final isFollowing = (response.data?['is_following'] as bool?) ?? false;
      return Success(isFollowing);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<FollowPage>> listFollowing(
    String userId, {
    String? cursor,
  }) async {
    try {
      final queryParams = <String, dynamic>{};
      if (cursor != null && cursor.isNotEmpty) {
        queryParams['cursor'] = cursor;
      }

      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/users/$userId/following',
        queryParameters: queryParams,
      );

      return Success(_parseFollowPage(response.data!));
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<FollowPage>> listFollowers(
    String userId, {
    String? cursor,
  }) async {
    try {
      final queryParams = <String, dynamic>{};
      if (cursor != null && cursor.isNotEmpty) {
        queryParams['cursor'] = cursor;
      }

      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/users/$userId/followers',
        queryParameters: queryParams,
      );

      return Success(_parseFollowPage(response.data!));
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  FollowPage _parseFollowPage(Map<String, dynamic> json) {
    final rawItems = json['items'];
    final items = rawItems is List
        ? rawItems
              .whereType<Map<String, dynamic>>()
              .map(_parseFollowUser)
              .toList()
        : <FollowUser>[];

    final nextCursorRaw = (json['next_cursor'] as String?) ?? '';
    return FollowPage(
      items: items,
      nextCursor: nextCursorRaw.isEmpty ? null : nextCursorRaw,
      terminated: (json['terminated'] as bool?) ?? false,
    );
  }

  FollowUser _parseFollowUser(Map<String, dynamic> json) {
    return FollowUser(
      id: json['id'] as String,
      handle: json['handle'] as String,
      displayName: json['display_name'] as String,
      avatarUrl: json['avatar_url'] as String?,
    );
  }
}
