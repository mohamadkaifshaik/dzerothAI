import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../../post/data/models/post_dto.dart';
import '../../../post/domain/entities/post.dart';
import '../../domain/entities/user_search_result.dart';
import '../../domain/repositories/search_repository.dart';
import '../models/user_search_result_model.dart';

/// Remote data source for search operations.
///
/// Calls:
///   GET /api/v1/search/posts?q={query}&cursor={cursor}
///   GET /api/v1/search/users?q={query}&cursor={cursor}
///
/// Auth is optional — the shared [Dio] instance attaches a token when
/// [AuthInterceptor] has one, but these endpoints accept unauthenticated
/// requests too.
class SearchRemoteDataSource {
  const SearchRemoteDataSource({required this.dio});

  final Dio dio;

  /// Fetches a page of post results for [query].
  Future<Result<SearchPostPage>> searchPosts({
    required String query,
    String? cursor,
  }) async {
    try {
      final queryParams = <String, dynamic>{'q': query};
      if (cursor != null && cursor.isNotEmpty) {
        queryParams['cursor'] = cursor;
      }

      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/search/posts',
        queryParameters: queryParams,
      );

      return Success(_parsePostPage(response.data!));
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  /// Fetches a page of user results for [query].
  Future<Result<SearchUserPage>> searchUsers({
    required String query,
    String? cursor,
  }) async {
    try {
      final queryParams = <String, dynamic>{'q': query};
      if (cursor != null && cursor.isNotEmpty) {
        queryParams['cursor'] = cursor;
      }

      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/search/users',
        queryParameters: queryParams,
      );

      return Success(_parseUserPage(response.data!));
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  SearchPostPage _parsePostPage(Map<String, dynamic> json) {
    final rawItems = json['items'];
    final List<Post> posts = rawItems is List
        ? rawItems
              .whereType<Map<String, dynamic>>()
              .map(PostDto.fromJson)
              .map((dto) => dto.toEntity())
              .toList()
        : const [];

    final nextCursorRaw = (json['next_cursor'] as String?) ?? '';
    return SearchPostPage(
      items: posts,
      nextCursor: nextCursorRaw.isEmpty ? null : nextCursorRaw,
      terminated: (json['terminated'] as bool?) ?? false,
    );
  }

  SearchUserPage _parseUserPage(Map<String, dynamic> json) {
    final rawItems = json['items'];
    final List<UserSearchResult> users = rawItems is List
        ? rawItems
              .whereType<Map<String, dynamic>>()
              .map(UserSearchResultModel.fromJson)
              .map((m) => m.toEntity())
              .toList()
        : const [];

    final nextCursorRaw = (json['next_cursor'] as String?) ?? '';
    return SearchUserPage(
      items: users,
      nextCursor: nextCursorRaw.isEmpty ? null : nextCursorRaw,
      terminated: (json['terminated'] as bool?) ?? false,
    );
  }
}
