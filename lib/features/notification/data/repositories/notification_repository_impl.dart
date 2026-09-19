import '../../../../core/error/result.dart';
import '../../domain/entities/notification.dart';
import '../../domain/repositories/notification_repository.dart';
import '../datasources/notification_remote_data_source.dart';

/// Concrete implementation of [NotificationRepository] backed by the Dzeroth
/// API via [NotificationRemoteDataSource].
class NotificationRepositoryImpl implements NotificationRepository {
  const NotificationRepositoryImpl({required this.dataSource});

  final NotificationRemoteDataSource dataSource;

  @override
  Future<Result<NotificationPage>> getNotifications({String? cursor}) {
    return dataSource.getNotifications(cursor: cursor);
  }

  @override
  Future<Result<void>> markAllRead() {
    return dataSource.markAllRead();
  }
}
