import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Thin wrapper around [FlutterSecureStorage] for Dzeroth token persistence.
///
/// Only the refresh token is persisted to disk. The access token is held
/// in-memory (BLoC state) per ADR 0005.
class SecureStorage {
  SecureStorage(this._storage);

  final FlutterSecureStorage _storage;

  static const String _refreshTokenKey = 'dzeroth_refresh_token';

  Future<void> writeRefreshToken(String token) =>
      _storage.write(key: _refreshTokenKey, value: token);

  Future<String?> readRefreshToken() => _storage.read(key: _refreshTokenKey);

  Future<void> deleteRefreshToken() => _storage.delete(key: _refreshTokenKey);
}
