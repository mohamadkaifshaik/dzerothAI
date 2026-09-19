import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/repositories/report_repository.dart';

/// Concrete implementation of [ReportRepository] backed by the Dzeroth API.
///
/// Uses the shared authenticated [Dio] instance provided via constructor
/// injection.
///
/// Endpoints:
///   POST /api/v1/posts/{postId}/report  → 204
///   POST /api/v1/users/{userId}/report  → 204
class ReportRepositoryImpl implements ReportRepository {
  const ReportRepositoryImpl({required this.dio});

  final Dio dio;

  @override
  Future<Result<void>> reportPost({
    required String postId,
    required String reason,
    String? detail,
  }) async {
    try {
      final body = <String, dynamic>{'reason': reason};
      if (detail != null && detail.isNotEmpty) {
        body['detail'] = detail;
      }
      await dio.post<void>('/api/v1/posts/$postId/report', data: body);
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<void>> reportUser({
    required String userId,
    required String reason,
    String? detail,
  }) async {
    try {
      final body = <String, dynamic>{'reason': reason};
      if (detail != null && detail.isNotEmpty) {
        body['detail'] = detail;
      }
      await dio.post<void>('/api/v1/users/$userId/report', data: body);
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }
}
