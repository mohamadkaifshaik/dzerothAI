import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/entities/analytics_page.dart';
import '../../domain/entities/post_analytics.dart';
import '../../domain/repositories/studio_repository.dart';
import '../models/post_analytics_model.dart';

/// Concrete implementation of [StudioRepository] backed by the Dzeroth API.
///
/// Uses the shared authenticated [Dio] instance provided via constructor
/// injection. The [AuthInterceptor] attached to [dio] supplies the Bearer token.
///
/// Endpoint:
///   GET /api/v1/me/studio/analytics?cursor={base64url-cursor}
///   Response: { items, next_cursor, terminated }
class StudioRepositoryImpl implements StudioRepository {
  const StudioRepositoryImpl({required this.dio});

  final Dio dio;

  @override
  Future<Result<AnalyticsPage>> getAnalytics({String? cursor}) async {
    try {
      final queryParams = <String, dynamic>{};
      if (cursor != null && cursor.isNotEmpty) {
        queryParams['cursor'] = cursor;
      }

      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/me/studio/analytics',
        queryParameters: queryParams,
      );

      return Success(_parseAnalyticsPage(response.data!));
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  AnalyticsPage _parseAnalyticsPage(Map<String, dynamic> json) {
    final rawItems = json['items'];
    final items = rawItems is List
        ? rawItems
              .whereType<Map<String, dynamic>>()
              .map(PostAnalyticsModel.fromJson)
              .map((m) => m.toEntity())
              .toList()
        : <PostAnalytics>[];

    final nextCursorRaw = (json['next_cursor'] as String?) ?? '';
    return AnalyticsPage(
      items: items,
      nextCursor: nextCursorRaw.isEmpty ? null : nextCursorRaw,
      terminated: (json['terminated'] as bool?) ?? false,
    );
  }
}
