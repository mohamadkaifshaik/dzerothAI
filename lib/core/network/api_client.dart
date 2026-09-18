import 'package:dio/dio.dart';

import '../config/app_config.dart';
import '../storage/secure_storage.dart';
import 'auth_interceptor.dart';

/// Constructs and configures the application-wide [Dio] instance.
///
/// A single instance should be created at startup and shared via DI.
/// The [AuthInterceptor] is attached here; its callbacks wire back to
/// AuthBloc via the [getAccessToken] / [onTokenRefreshed] / [onUnauthenticated]
/// parameters.
Dio createApiClient({
  required SecureStorage secureStorage,
  required String? Function() getAccessToken,
  required void Function(String accessToken, String refreshToken)
  onTokenRefreshed,
  required void Function() onUnauthenticated,
}) {
  final dio = Dio(
    BaseOptions(
      baseUrl: AppConfig.apiBaseUrl,
      connectTimeout: const Duration(seconds: 10),
      receiveTimeout: const Duration(seconds: 30),
      headers: {'Content-Type': 'application/json'},
    ),
  );

  dio.interceptors.add(
    AuthInterceptor(
      getAccessToken: getAccessToken,
      onTokenRefreshed: onTokenRefreshed,
      onUnauthenticated: onUnauthenticated,
      secureStorage: secureStorage,
      refreshBaseUrl: AppConfig.apiBaseUrl,
    ),
  );

  return dio;
}
