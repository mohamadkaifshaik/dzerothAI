import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/entities/own_profile.dart';
import '../../domain/entities/profile.dart';
import '../../domain/repositories/profile_repository.dart';
import '../models/own_profile_dto.dart';
import '../models/profile_dto.dart';

/// Concrete implementation of [ProfileRepository] backed by the Dzeroth API.
class ProfileRepositoryImpl implements ProfileRepository {
  ProfileRepositoryImpl({required this.dio});

  final Dio dio;

  @override
  Future<Result<OwnProfile>> getOwnProfile() async {
    try {
      final response = await dio.get<Map<String, dynamic>>('/api/v1/me');
      final dto = OwnProfileDto.fromJson(response.data!);
      return Success(dto.toDomain());
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<OwnProfile>> updateOwnProfile({
    String? displayName,
    String? bio,
    String? location,
    String? websiteUrl,
  }) async {
    try {
      final body = <String, dynamic>{};
      if (displayName != null) body['display_name'] = displayName;
      if (bio != null) body['bio'] = bio;
      if (location != null) body['location'] = location;
      if (websiteUrl != null) body['website_url'] = websiteUrl;

      final response = await dio.put<Map<String, dynamic>>(
        '/api/v1/me',
        data: body,
      );
      final dto = OwnProfileDto.fromJson(response.data!);
      return Success(dto.toDomain());
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  @override
  Future<Result<Profile>> getUserProfile(String userId) async {
    try {
      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/users/$userId',
      );
      final dto = ProfileDto.fromJson(response.data!);
      return Success(dto.toDomain());
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }
}
