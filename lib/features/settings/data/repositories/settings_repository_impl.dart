import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/repositories/settings_repository.dart';

/// Concrete implementation of [SettingsRepository] backed by the Dzeroth API.
class SettingsRepositoryImpl implements SettingsRepository {
  const SettingsRepositoryImpl({required this.dio});

  final Dio dio;

  @override
  Future<Result<SettingsData>> getSettings() async {
    try {
      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/me/settings',
      );
      final data = _parseResponseData(response.data!);
      return Success(
        SettingsData(isPrivate: (data['is_private'] as bool?) ?? false),
      );
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<SettingsData>> updatePrivacy({required bool isPrivate}) async {
    try {
      final response = await dio.put<Map<String, dynamic>>(
        '/api/v1/me/settings',
        data: {'is_private': isPrivate},
      );
      final data = _parseResponseData(response.data!);
      return Success(
        SettingsData(isPrivate: (data['is_private'] as bool?) ?? isPrivate),
      );
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<void>> suspendAccount() async {
    try {
      await dio.delete<void>('/api/v1/me/account');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  /// Unwraps the single-resource envelope `{"data": {...}}` if present,
  /// otherwise returns the response body directly.
  Map<String, dynamic> _parseResponseData(Map<String, dynamic> body) {
    final data = body['data'];
    if (data is Map<String, dynamic>) return data;
    return body;
  }
}
