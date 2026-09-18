import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/repositories/block_repository.dart';

/// Concrete implementation of [BlockRepository] backed by the Dzeroth API.
///
/// Uses the shared authenticated [Dio] instance provided via constructor
/// injection.
///
/// Endpoints:
///   POST   /api/v1/users/{userId}/block   → 204
///   DELETE /api/v1/users/{userId}/block   → 204
///   POST   /api/v1/users/{userId}/mute    → 204
///   DELETE /api/v1/users/{userId}/mute    → 204
class BlockRepositoryImpl implements BlockRepository {
  const BlockRepositoryImpl({required this.dio});

  final Dio dio;

  @override
  Future<Result<void>> blockUser(String targetUserId) async {
    try {
      await dio.post<void>('/api/v1/users/$targetUserId/block');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<void>> unblockUser(String targetUserId) async {
    try {
      await dio.delete<void>('/api/v1/users/$targetUserId/block');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<void>> muteUser(String targetUserId) async {
    try {
      await dio.post<void>('/api/v1/users/$targetUserId/mute');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<void>> unmuteUser(String targetUserId) async {
    try {
      await dio.delete<void>('/api/v1/users/$targetUserId/mute');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }
}
