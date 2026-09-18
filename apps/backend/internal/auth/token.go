// Package auth implements authentication token generation and validation.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims is the set of JWT claims used by Dzeroth access tokens.
// The authenticated user UUID is carried in the standard "sub" claim via
// RegisteredClaims.Subject. JTI (jti) is a UUID v4 audit identifier; it is
// not a foreign key to any table. Per ADR 0005 there are no role claims.
type Claims struct {
	jwt.RegisteredClaims
}

const accessTokenLifetime = 15 * time.Minute

// GenerateAccessToken creates a signed HS256 JWT for the given user.
// The token expires in 15 minutes and carries a UUID v4 jti for audit purposes.
func GenerateAccessToken(userID uuid.UUID, secret []byte) (string, error) {
	now := time.Now().UTC()
	jti := uuid.New() // UUID v4 per ADR 0004/0005

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenLifetime)),
			ID:        jti.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign access token: %w", err)
	}
	return signed, nil
}

// ValidateAccessToken parses and verifies a JWT string. It checks the HS256 signature
// and the expiry claim. On success it returns the parsed Claims; on any failure it
// returns a descriptive error (callers must not expose this error text to clients).
func ValidateAccessToken(tokenStr string, secret []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenStr,
		&Claims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("auth: unexpected signing method: %v", t.Header["alg"])
			}
			return secret, nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("auth: validate token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("auth: invalid token claims")
	}
	return claims, nil
}

// GenerateRefreshToken creates a cryptographically random 32-byte refresh token.
// It returns:
//   - raw: the hex-encoded value that is returned to the client exactly once.
//   - hash: the SHA-256 hash of the raw bytes, which is stored in the sessions table.
//
// The raw value is never persisted. Only the hash is written to the database.
func GenerateRefreshToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("auth: generate refresh token: %w", err)
	}
	raw = hex.EncodeToString(b)
	sum := sha256.Sum256(b)
	hash = hex.EncodeToString(sum[:])
	return raw, hash, nil
}

// HashRefreshToken returns the SHA-256 hex hash of a hex-encoded raw refresh token.
// Used when verifying a client-supplied refresh token against the stored hash.
func HashRefreshToken(rawHex string) (string, error) {
	b, err := hex.DecodeString(rawHex)
	if err != nil {
		return "", fmt.Errorf("auth: decode refresh token: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
