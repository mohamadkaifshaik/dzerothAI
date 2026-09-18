import 'package:dio/dio.dart';

import 'failures.dart';

/// Models the Go API error envelope:
/// ```json
/// {
///   "error": {
///     "code": "VALIDATION_ERROR",
///     "message": "Human-readable description.",
///     "details": [{ "field": "handle", "message": "must be 3–50 characters" }]
///   }
/// }
/// ```
class ApiErrorBody {
  const ApiErrorBody({
    required this.code,
    required this.message,
    this.details = const [],
  });

  final String code;
  final String message;
  final List<FieldError> details;

  factory ApiErrorBody.fromJson(Map<String, dynamic> json) {
    final errorMap = json['error'] as Map<String, dynamic>?;
    if (errorMap == null) {
      return const ApiErrorBody(
        code: 'UNKNOWN',
        message: 'An unknown error occurred.',
      );
    }
    final rawDetails = errorMap['details'];
    final details = rawDetails is List
        ? rawDetails
              .whereType<Map<String, dynamic>>()
              .map(FieldError.fromJson)
              .toList()
        : <FieldError>[];

    return ApiErrorBody(
      code: (errorMap['code'] as String?) ?? 'UNKNOWN',
      message: (errorMap['message'] as String?) ?? 'An unknown error occurred.',
      details: details,
    );
  }
}

/// Maps a [DioException] to a domain [Failure].
///
/// This is the single place that knows about HTTP status codes in the
/// data layer. Everything above this function works with [Failure] subtypes.
Failure mapDioError(DioException e) {
  switch (e.type) {
    case DioExceptionType.connectionTimeout:
    case DioExceptionType.sendTimeout:
    case DioExceptionType.receiveTimeout:
    case DioExceptionType.connectionError:
      return const NetworkFailure();

    case DioExceptionType.badResponse:
      final response = e.response;
      if (response == null) return const ServerFailure();

      ApiErrorBody? body;
      try {
        final data = response.data;
        if (data is Map<String, dynamic>) {
          body = ApiErrorBody.fromJson(data);
        }
      } catch (_) {
        // Ignore parse errors — fall through to status-based defaults.
      }

      switch (response.statusCode) {
        case 400:
          return ValidationFailure(
            message: body?.message ?? 'Bad request.',
            details: body?.details ?? [],
          );
        case 401:
          return UnauthorizedFailure(body?.message ?? 'You are not signed in.');
        case 403:
          return ForbiddenFailure(
            body?.message ?? 'You do not have permission.',
          );
        case 404:
          return NotFoundFailure(body?.message ?? 'Resource not found.');
        case 409:
          return ConflictFailure(body?.message ?? 'A conflict occurred.');
        case 422:
          return ValidationFailure(
            message: body?.message ?? 'Please correct the highlighted fields.',
            details: body?.details ?? [],
          );
        case 429:
          return RateLimitFailure(
            body?.message ?? 'Too many requests. Please wait.',
          );
        case 503:
          return ServiceUnavailableFailure(
            body?.message ?? 'Service is temporarily unavailable.',
          );
        default:
          return ServerFailure(
            body?.message ?? 'An unexpected server error occurred.',
          );
      }

    case DioExceptionType.cancel:
      return const NetworkFailure('Request was cancelled.');

    case DioExceptionType.unknown:
    case DioExceptionType.badCertificate:
    case DioExceptionType.transformTimeout:
      return const NetworkFailure();
  }
}
