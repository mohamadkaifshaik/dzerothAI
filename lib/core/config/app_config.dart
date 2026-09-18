/// Application-wide configuration constants.
///
/// Values are provided via Dart compile-time environment variables.
/// Supply them with `--dart-define=API_BASE_URL=https://api.example.com`
/// at build or run time.
class AppConfig {
  AppConfig._();

  static const String apiBaseUrl = String.fromEnvironment(
    'API_BASE_URL',
    defaultValue: 'http://localhost:8080',
  );
}
