import '../../../../core/error/result.dart';
import '../entities/user_title.dart';

/// Contract for the title data source.
///
/// Authentication is required for all `me` endpoints.
/// The implementation must rely on the shared authenticated [Dio] instance
/// (Bearer token injected by [AuthInterceptor]).
///
/// GET /titles/{userId}/primary is public (auth optional).
abstract class TitleRepository {
  /// Fetch all titles owned by the authenticated user.
  ///
  /// Returns items with a [primaryId] indicating which title is currently
  /// set as primary (null when no primary title is set).
  Future<Result<MyTitlesResult>> getMyTitles();

  /// Set the authenticated user's primary title.
  ///
  /// [userTitleId] is the `id` of a [UserTitle] from [getMyTitles].
  /// Returns the updated primary title's [UserTitle.id], or null on clear.
  Future<Result<String?>> setPrimaryTitle(String userTitleId);

  /// Clear the authenticated user's primary title.
  Future<Result<void>> clearPrimaryTitle();

  /// Fetch the primary title for any user by their [userId].
  ///
  /// Auth is optional — the implementation passes the token when available
  /// but the endpoint is accessible unauthenticated.
  Future<Result<TitleSummary?>> getUserPrimaryTitle(String userId);
}

/// Result envelope for [TitleRepository.getMyTitles].
class MyTitlesResult {
  const MyTitlesResult({required this.titles, required this.primaryId});

  final List<UserTitle> titles;

  /// The [UserTitle.id] of the currently active primary title, or null.
  final String? primaryId;
}
