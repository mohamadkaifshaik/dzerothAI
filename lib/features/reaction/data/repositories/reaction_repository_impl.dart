import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/repositories/reaction_repository.dart';

/// Concrete implementation of [ReactionRepository] backed by the Dzeroth API.
///
/// Uses the shared authenticated [Dio] instance provided via constructor
/// injection.
///
/// Endpoints:
///   POST   /api/v1/posts/{postId}/react   → 204
///   DELETE /api/v1/posts/{postId}/react   → 204
class ReactionRepositoryImpl implements ReactionRepository {
  const ReactionRepositoryImpl({required this.dio});

  final Dio dio;

  @override
  Future<Result<void>> react(String postId) async {
    try {
      await dio.post<void>('/api/v1/posts/$postId/react');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<void>> unreact(String postId) async {
    try {
      await dio.delete<void>('/api/v1/posts/$postId/react');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }
}
