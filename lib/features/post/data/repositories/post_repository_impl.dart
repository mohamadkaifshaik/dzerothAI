import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/entities/post.dart';
import '../../domain/repositories/post_repository.dart';
import '../models/post_dto.dart';

/// Concrete implementation of [PostRepository] backed by the Dzeroth API.
///
/// Uses the shared authenticated [Dio] instance provided via constructor
/// injection.  The [AuthInterceptor] attached to that [Dio] handles token
/// attachment and silent refresh transparently.
class PostRepositoryImpl implements PostRepository {
  PostRepositoryImpl({required this.dio});

  final Dio dio;

  // ---------------------------------------------------------------------------
  // Write
  // ---------------------------------------------------------------------------

  @override
  Future<Result<Post>> createPost({
    required String postType,
    String? content,
    String? parentId,
    String? quotedPostId,
  }) async {
    try {
      final body = <String, dynamic>{'post_type': postType};
      if (content != null) body['content'] = content;
      if (parentId != null) body['parent_id'] = parentId;
      if (quotedPostId != null) body['quoted_post_id'] = quotedPostId;

      final response = await dio.post<Map<String, dynamic>>(
        '/api/v1/posts',
        data: body,
      );

      // The API wraps the created post in { "post": { ... } }.
      final data = response.data!;
      final postJson = data['post'] is Map<String, dynamic>
          ? data['post'] as Map<String, dynamic>
          : data;
      final dto = PostDto.fromJson(postJson);
      return Success(dto.toEntity());
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<void>> deletePost(String postId) async {
    try {
      await dio.delete<void>('/api/v1/posts/$postId');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  // ---------------------------------------------------------------------------
  // Read — single post
  // ---------------------------------------------------------------------------

  @override
  Future<Result<Post>> getPost(String postId) async {
    try {
      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/posts/$postId',
      );

      final data = response.data!;
      final postJson = data['post'] is Map<String, dynamic>
          ? data['post'] as Map<String, dynamic>
          : data;
      final dto = PostDto.fromJson(postJson);
      return Success(dto.toEntity());
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  // ---------------------------------------------------------------------------
  // Read — paginated feeds
  // ---------------------------------------------------------------------------

  @override
  Future<Result<PostPage>> listPostsByAuthor(
    String authorId, {
    String? cursor,
  }) async {
    try {
      final queryParams = <String, dynamic>{};
      if (cursor != null && cursor.isNotEmpty) {
        queryParams['cursor'] = cursor;
      }

      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/users/$authorId/posts',
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

  @override
  Future<Result<PostPage>> listThreadReplies(
    String threadRootId, {
    String? cursor,
  }) async {
    try {
      final queryParams = <String, dynamic>{};
      if (cursor != null && cursor.isNotEmpty) {
        queryParams['cursor'] = cursor;
      }

      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/posts/$threadRootId/thread',
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

  // ---------------------------------------------------------------------------
  // Helpers
  // ---------------------------------------------------------------------------

  PostPage _pageToEntity(PostPageDto dto) {
    return PostPage(
      items: dto.items.map((d) => d.toEntity()).toList(),
      nextCursor: dto.nextCursor.isEmpty ? null : dto.nextCursor,
      terminated: dto.terminated,
    );
  }
}
