import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../../post/data/models/post_dto.dart';
import '../../../post/domain/entities/post.dart';
import '../../domain/repositories/feed_repository.dart';

/// Concrete implementation of [FeedRepository] backed by the Dzeroth API.
///
/// Uses the shared authenticated [Dio] instance provided via constructor
/// injection.  The [AuthInterceptor] handles token attachment and silent
/// refresh transparently.
///
/// Endpoint: GET /api/v1/feeds/home
/// Query params: cursor (optional)
/// Response: { "items": [...], "next_cursor": "...", "terminated": bool }
class FeedRepositoryImpl implements FeedRepository {
  const FeedRepositoryImpl({required this.dio});

  final Dio dio;

  @override
  Future<Result<PostPage>> getHomeFeed({String? cursor}) async {
    try {
      final queryParams = <String, dynamic>{};
      if (cursor != null && cursor.isNotEmpty) {
        queryParams['cursor'] = cursor;
      }

      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/feeds/home',
        queryParameters: queryParams,
      );

      final pageDto = PostPageDto.fromJson(response.data!);
      return Success(_pageToEntity(pageDto));
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  PostPage _pageToEntity(PostPageDto dto) {
    return PostPage(
      items: dto.items.map((d) => d.toEntity()).toList(),
      nextCursor: dto.nextCursor.isEmpty ? null : dto.nextCursor,
      terminated: dto.terminated,
    );
  }
}
