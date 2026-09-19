import '../../../../core/error/result.dart';
import '../entities/analytics_page.dart';

/// Contract for the Creator Studio analytics data source.
///
/// Authentication is required — the implementation must rely on the shared
/// authenticated [Dio] instance (Bearer token injected by [AuthInterceptor]).
///
/// Per CLAUDE.md §2.3, the analytics data returned here is PRIVATE.
/// It must only be consumed by [StudioBloc] / [StudioScreen].
abstract class StudioRepository {
  /// Fetch a page of post analytics for the authenticated user.
  ///
  /// [cursor] is a base64url cursor string; pass null for the first page.
  Future<Result<AnalyticsPage>> getAnalytics({String? cursor});
}
