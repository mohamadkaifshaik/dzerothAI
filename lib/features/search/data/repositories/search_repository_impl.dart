import 'package:dio/dio.dart';

import '../../../../core/error/result.dart';
import '../../domain/repositories/search_repository.dart';
import '../datasources/search_remote_data_source.dart';

/// Concrete implementation of [SearchRepository] backed by the Dzeroth API.
///
/// Uses the shared [Dio] instance provided via constructor injection.
/// The [AuthInterceptor] attaches a token when available, but search
/// endpoints are auth-optional.
class SearchRepositoryImpl implements SearchRepository {
  SearchRepositoryImpl({required Dio dio})
    : _dataSource = SearchRemoteDataSource(dio: dio);

  final SearchRemoteDataSource _dataSource;

  @override
  Future<Result<SearchPostPage>> searchPosts({
    required String query,
    String? cursor,
  }) => _dataSource.searchPosts(query: query, cursor: cursor);

  @override
  Future<Result<SearchUserPage>> searchUsers({
    required String query,
    String? cursor,
  }) => _dataSource.searchUsers(query: query, cursor: cursor);
}
