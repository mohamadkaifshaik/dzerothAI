import 'package:dio/dio.dart';

import '../storage/secure_storage.dart';

/// Dio interceptor that attaches access tokens and handles silent refresh.
///
/// Access token lifecycle per ADR 0005:
/// - Stored in memory only (BLoC state). Retrieved via [getAccessToken] callback.
/// - On 401: attempt silent refresh using the stored refresh token.
/// - If refresh succeeds: [onTokenRefreshed] is called with the new pair,
///   and the original request is retried once.
/// - If refresh fails: [onUnauthenticated] is called so AuthBloc can
///   transition to the unauthenticated state.
class AuthInterceptor extends Interceptor {
  AuthInterceptor({
    required this.getAccessToken,
    required this.onTokenRefreshed,
    required this.onUnauthenticated,
    required this.secureStorage,
    required this.refreshBaseUrl,
  });

  /// Returns the current in-memory access token (may be null before first login).
  final String? Function() getAccessToken;

  /// Called when a new token pair is obtained from the refresh endpoint.
  final void Function(String accessToken, String refreshToken) onTokenRefreshed;

  /// Called when a silent refresh fails (session expired / revoked).
  final void Function() onUnauthenticated;

  final SecureStorage secureStorage;

  /// Base URL used for the refresh request (same as apiBaseUrl).
  final String refreshBaseUrl;

  static const String _retryHeader = 'X-Dzeroth-Retry';

  @override
  void onRequest(RequestOptions options, RequestInterceptorHandler handler) {
    final token = getAccessToken();
    if (token != null && token.isNotEmpty) {
      options.headers['Authorization'] = 'Bearer $token';
    }
    handler.next(options);
  }

  @override
  Future<void> onError(
    DioException err,
    ErrorInterceptorHandler handler,
  ) async {
    final response = err.response;

    // Only attempt refresh on 401, and only once per request.
    if (response?.statusCode != 401 ||
        err.requestOptions.headers.containsKey(_retryHeader)) {
      handler.next(err);
      return;
    }

    final refreshToken = await secureStorage.readRefreshToken();
    if (refreshToken == null) {
      onUnauthenticated();
      handler.next(err);
      return;
    }

    try {
      final refreshDio = Dio(
        BaseOptions(
          baseUrl: refreshBaseUrl,
          connectTimeout: const Duration(seconds: 10),
          receiveTimeout: const Duration(seconds: 30),
          headers: {'Content-Type': 'application/json'},
        ),
      );

      final refreshResponse = await refreshDio.post<Map<String, dynamic>>(
        '/api/v1/auth/refresh',
        data: {'refresh_token': refreshToken},
      );

      final data = refreshResponse.data;
      if (data == null) {
        onUnauthenticated();
        handler.next(err);
        return;
      }

      final newAccess = data['access_token'] as String?;
      final newRefresh = data['refresh_token'] as String?;

      if (newAccess == null || newRefresh == null) {
        onUnauthenticated();
        handler.next(err);
        return;
      }

      await secureStorage.writeRefreshToken(newRefresh);
      onTokenRefreshed(newAccess, newRefresh);

      // Retry the original request with the new access token.
      final retryOptions = err.requestOptions
        ..headers['Authorization'] = 'Bearer $newAccess'
        ..headers[_retryHeader] = '1';

      final retryDio = Dio(
        BaseOptions(
          baseUrl: refreshBaseUrl,
          connectTimeout: const Duration(seconds: 10),
          receiveTimeout: const Duration(seconds: 30),
        ),
      );

      final retryResponse = await retryDio.fetch<dynamic>(retryOptions);
      handler.resolve(retryResponse);
    } catch (_) {
      await secureStorage.deleteRefreshToken();
      onUnauthenticated();
      handler.next(err);
    }
  }
}
