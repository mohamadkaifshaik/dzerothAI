/// DTO for the token pair returned by login, register, and refresh endpoints.
///
/// API shape (wrapped in the standard single-resource envelope):
/// ```json
/// {
///   "data": {
///     "access_token": "eyJ...",
///     "refresh_token": "abc123..."
///   }
/// }
/// ```
class TokenPairDto {
  const TokenPairDto({required this.accessToken, required this.refreshToken});

  final String accessToken;
  final String refreshToken;

  factory TokenPairDto.fromJson(Map<String, dynamic> json) {
    return TokenPairDto(
      accessToken: json['access_token'] as String,
      refreshToken: json['refresh_token'] as String,
    );
  }
}

/// DTO for the register / login response.
///
/// The JWT `sub` claim encodes the user ID. The API also returns it
/// directly in the response body to spare the client from JWT parsing.
class AuthResponseDto {
  const AuthResponseDto({required this.tokens, required this.userId});

  final TokenPairDto tokens;
  final String userId;

  factory AuthResponseDto.fromJson(Map<String, dynamic> json) {
    // Unwrap the standard single-resource envelope: { "data": { ... } }
    final data = json['data'] as Map<String, dynamic>? ?? json;
    return AuthResponseDto(
      tokens: TokenPairDto.fromJson(data),
      userId: data['user_id'] as String,
    );
  }
}
