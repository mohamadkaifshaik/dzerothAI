import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../../post/data/models/post_dto.dart';
import '../../domain/entities/bookmark_item.dart';
import '../../domain/entities/bookmark_page.dart';
import '../../domain/repositories/bookmark_repository.dart';

/// Concrete implementation of [BookmarkRepository] backed by the Dzeroth API.
///
/// Uses the shared authenticated [Dio] instance provided via constructor
/// injection.
///
/// Endpoints:
///   POST   /api/v1/posts/{postId}/bookmark   → 204
///   DELETE /api/v1/posts/{postId}/bookmark   → 204
///   GET    /api/v1/me/bookmarks              → { items, next_cursor, terminated }
class BookmarkRepositoryImpl implements BookmarkRepository {
  const BookmarkRepositoryImpl({required this.dio});

  final Dio dio;

  @override
  Future<Result<void>> bookmark(String postId) async {
    try {
      await dio.post<void>('/api/v1/posts/$postId/bookmark');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<void>> unbookmark(String postId) async {
    try {
      await dio.delete<void>('/api/v1/posts/$postId/bookmark');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<BookmarkPage>> listBookmarks({String? cursor}) async {
    try {
      final queryParams = <String, dynamic>{};
      if (cursor != null && cursor.isNotEmpty) {
        queryParams['cursor'] = cursor;
      }

      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/me/bookmarks',
        queryParameters: queryParams,
      );

      return Success(_parseBookmarkPage(response.data!));
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  BookmarkPage _parseBookmarkPage(Map<String, dynamic> json) {
    final rawItems = json['items'];
    final items = rawItems is List
        ? rawItems
              .whereType<Map<String, dynamic>>()
              .map(_parseBookmarkItem)
              .toList()
        : <BookmarkItem>[];

    final nextCursorRaw = (json['next_cursor'] as String?) ?? '';
    return BookmarkPage(
      items: items,
      nextCursor: nextCursorRaw.isEmpty ? null : nextCursorRaw,
      terminated: (json['terminated'] as bool?) ?? false,
    );
  }

  BookmarkItem _parseBookmarkItem(Map<String, dynamic> json) {
    final postRaw = json['post'] as Map<String, dynamic>;
    final post = PostDto.fromJson(postRaw).toEntity();
    return BookmarkItem(
      postId: json['post_id'] as String,
      createdAt: DateTime.parse(json['created_at'] as String),
      post: post,
    );
  }
}
