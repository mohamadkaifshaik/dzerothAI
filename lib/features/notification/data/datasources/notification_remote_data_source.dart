import 'package:dio/dio.dart';

import '../../../../core/error/api_error.dart';
import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/entities/notification.dart';
import '../models/notification_model.dart';

/// Remote data source for notification operations.
///
/// Endpoints:
///   GET /api/v1/me/notifications?cursor=  — auth required, cursor-paginated
///   PUT /api/v1/me/notifications/read     — auth required, 204
class NotificationRemoteDataSource {
  const NotificationRemoteDataSource({required this.dio});

  final Dio dio;

  /// Fetches a page of notifications for the authenticated user.
  ///
  /// The backend terminates at 100 items. When [NotificationPage.terminated]
  /// is true the caller must stop fetching further pages.
  Future<Result<NotificationPage>> getNotifications({String? cursor}) async {
    try {
      final queryParams = <String, dynamic>{};
      if (cursor != null && cursor.isNotEmpty) {
        queryParams['cursor'] = cursor;
      }

      final response = await dio.get<Map<String, dynamic>>(
        '/api/v1/me/notifications',
        queryParameters: queryParams,
      );

      return Success(_parseNotificationPage(response.data!));
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  /// Marks all notifications for the authenticated user as read (PUT 204).
  Future<Result<void>> markAllRead() async {
    try {
      await dio.put<void>('/api/v1/me/notifications/read');
      return const Success(null);
    } on DioException catch (e) {
      return Err(mapDioError(e));
    } catch (_) {
      return const Err(ServerFailure());
    }
  }

  NotificationPage _parseNotificationPage(Map<String, dynamic> json) {
    final rawItems = json['items'];
    final items = rawItems is List
        ? rawItems
              .whereType<Map<String, dynamic>>()
              .map(NotificationModel.fromJson)
              .map((m) => m.toEntity())
              .toList()
        : <Notification>[];

    final nextCursorRaw = (json['next_cursor'] as String?) ?? '';
    return NotificationPage(
      items: items,
      nextCursor: nextCursorRaw.isEmpty ? null : nextCursorRaw,
      terminated: (json['terminated'] as bool?) ?? false,
    );
  }
}
