import 'package:equatable/equatable.dart';

/// Base domain failure type.
///
/// All concrete failures extend this class. Failures are domain-level:
/// they must not contain HTTP status codes or Dio-specific types.
sealed class Failure extends Equatable {
  const Failure(this.message);

  final String message;

  @override
  List<Object?> get props => [message];
}

/// The device could not reach the server (no connectivity, DNS, timeout, etc.).
final class NetworkFailure extends Failure {
  const NetworkFailure([super.message = 'No network connection.']);
}

/// The request was rejected because the caller is not authenticated (HTTP 401).
final class UnauthorizedFailure extends Failure {
  const UnauthorizedFailure([super.message = 'You are not signed in.']);
}

/// The caller is authenticated but lacks permission for this resource (HTTP 403).
final class ForbiddenFailure extends Failure {
  const ForbiddenFailure([super.message = 'You do not have permission.']);
}

/// The requested resource does not exist (HTTP 404).
final class NotFoundFailure extends Failure {
  const NotFoundFailure([super.message = 'Resource not found.']);
}

/// The request conflicts with existing state (HTTP 409).
/// Typical usage: duplicate handle/email at registration.
final class ConflictFailure extends Failure {
  const ConflictFailure([super.message = 'A conflict occurred.']);
}

/// Input failed server-side validation (HTTP 422).
final class ValidationFailure extends Failure {
  const ValidationFailure({
    String message = 'Please correct the highlighted fields.',
    this.details = const [],
  }) : super(message);

  final List<FieldError> details;

  @override
  List<Object?> get props => [message, details];
}

/// Too many requests (HTTP 429).
final class RateLimitFailure extends Failure {
  const RateLimitFailure([super.message = 'Too many requests. Please wait.']);
}

/// Unexpected server error (HTTP 500).
final class ServerFailure extends Failure {
  const ServerFailure([super.message = 'An unexpected server error occurred.']);
}

/// The service is temporarily unavailable (HTTP 503).
final class ServiceUnavailableFailure extends Failure {
  const ServiceUnavailableFailure([
    super.message = 'Service is temporarily unavailable.',
  ]);
}

/// A field-level validation error from the server.
final class FieldError extends Equatable {
  const FieldError({required this.field, required this.message});

  final String field;
  final String message;

  factory FieldError.fromJson(Map<String, dynamic> json) {
    return FieldError(
      field: (json['field'] as String?) ?? '',
      message: (json['message'] as String?) ?? '',
    );
  }

  @override
  List<Object?> get props => [field, message];
}
