import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/entities/user_title.dart';
import '../../domain/repositories/title_repository.dart';
import '../models/title_dto.dart';

/// Concrete implementation of [TitleRepository] backed by the Dzeroth API.
///
/// Uses the shared authenticated [Dio] instance provided via constructor
/// injection. The [AuthInterceptor] attached to [dio] supplies the Bearer token
/// for authenticated endpoints.
///
/// Endpoints:
///   GET    /api/v1/titles/me              → MyTitlesResult
///   PUT    /api/v1/titles/me/primary      → updated primary id
///   DELETE /api/v1/titles/me/primary      → 204 no content
///   GET    /api/v1/titles/{userId}/primary → TitleSummary?
class TitleRepositoryImpl implements TitleRepository {
  const TitleRepositoryImpl({required this.dio});

  final Dio dio;

  @override
  Future<Result<MyTitlesResult>> getMyTitles() async {
    try {
      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/titles/me',
      );

      final data = response.data!;
      final rawItems = data['items'];
      final items = rawItems is List
          ? rawItems
                .whereType<Map<String, dynamic>>()
                .map(UserTitleDto.fromJson)
                .map((dto) => dto.toEntity())
                .toList()
          : <UserTitle>[];

      final primaryId = data['primary_id'] as String?;
      final normalizedPrimaryId =
          (primaryId != null && primaryId.isEmpty) ? null : primaryId;

      return Success(
        MyTitlesResult(titles: items, primaryId: normalizedPrimaryId),
      );
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<String?>> setPrimaryTitle(String userTitleId) async {
    try {
      final response = await dio.put<Map<String, dynamic>>(
        '/api/v1/titles/me/primary',
        data: {'user_title_id': userTitleId},
      );

      final data = response.data;
      final primaryTitleMap = data?['primary_title'] as Map<String, dynamic>?;

      // The backend echoes back the new primary title; we extract the slug
      // as a stable identifier. If absent, return null.
      final slug = primaryTitleMap?['slug'] as String?;
      return Success(slug);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<void>> clearPrimaryTitle() async {
    try {
      await dio.delete<void>('/api/v1/titles/me/primary');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<TitleSummary?>> getUserPrimaryTitle(String userId) async {
    try {
      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/titles/$userId/primary',
      );

      final data = response.data;
      final rawTitle = data?['primary_title'];
      if (rawTitle == null) return const Success(null);

      if (rawTitle is Map<String, dynamic>) {
        return Success(TitleSummaryDto.fromJson(rawTitle).toEntity());
      }
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }
}
