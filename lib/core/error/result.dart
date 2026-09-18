import 'failures.dart';

/// A simple discriminated union for operation results.
///
/// Use [Success] when the operation succeeded and [Err] when it failed.
/// This avoids a dependency on `dartz` (not in pubspec) while keeping
/// callers honest about handling both outcomes.
sealed class Result<T> {
  const Result();
}

final class Success<T> extends Result<T> {
  const Success(this.value);

  final T value;
}

final class Err<T> extends Result<T> {
  const Err(this.failure);

  final Failure failure;
}
