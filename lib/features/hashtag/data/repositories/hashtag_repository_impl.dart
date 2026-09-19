import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../../post/data/models/post_dto.dart';
import '../../../post/domain/entities/post.dart';
import '../../domain/repositories/hashtag_repository.dart';

/// Concrete implementation of [HashtagRepository] backed by the Dzeroth API.
///
/// Calls `GET /api/v1/hashtags/{tag}/posts` — auth optional.
/// The shared [Dio] instance attaches a token when the user is authenticated;
/// unauthenticated callers receive results without block filtering.
class HashtagRepositoryImpl implements HashtagRepository {
  HashtagRepositoryImpl({required this._dio});

  final Dio _dio;

  @override
  Future<Result<PostPage>> getHashtagFeed({
    required String tag,
    String? cursor,
  }) async {
    try {
      // Strip leading '#' so the URL segment is clean.
      final normalizedTag = tag.startsWith('#') ? tag.substring(1) : tag;

      final queryParams = <String, dynamic>{};
      if (cursor != null && cursor.isNotEmpty) {
        queryParams['cursor'] = cursor;
      }

      final response = await _dio.get<Map<String, dynamic>>(
        '/api/v1/hashtags/$normalizedTag/posts',
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
