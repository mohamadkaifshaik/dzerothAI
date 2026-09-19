package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var testSecret = []byte("test-secret-that-is-at-least-32-bytes-long!")

// ---------------------------------------------------------------------------
// GenerateAccessToken / ValidateAccessToken
// ---------------------------------------------------------------------------

func TestGenerateAndValidateAccessToken_ValidToken(t *testing.T) {
	userID := uuid.New()
	sessionID := uuid.New()
	tokenStr, err := GenerateAccessToken(userID, sessionID, testSecret)
	if err != nil {
		t.Fatalf("GenerateAccessToken returned unexpected error: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("GenerateAccessToken returned empty token string")
	}

	claims, err := ValidateAccessToken(tokenStr, testSecret)
	if err != nil {
		t.Fatalf("ValidateAccessToken rejected a freshly generated valid token: %v", err)
	}

	if claims.Subject != userID.String() {
		t.Errorf("claims.Subject = %q, want %q", claims.Subject, userID.String())
	}
	if claims.SessionID != sessionID {
		t.Errorf("claims.SessionID = %v, want %v", claims.SessionID, sessionID)
	}
}

func TestValidateAccessToken_ExpiredTokenIsRejected(t *testing.T) {
	userID := uuid.New()
	jti := uuid.New()
	now := time.Now().UTC()

	// Craft a token with a past expiry.
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now.Add(-30 * time.Minute)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-15 * time.Minute)), // already expired
			ID:        jti.String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(testSecret)
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}

	_, err = ValidateAccessToken(signed, testSecret)
	if err == nil {
		t.Fatal("ValidateAccessToken accepted an expired token — expected an error")
	}
}

func TestValidateAccessToken_WrongSignatureIsRejected(t *testing.T) {
	userID := uuid.New()
	tokenStr, err := GenerateAccessToken(userID, uuid.New(), testSecret)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	// Use a different secret to validate — should fail.
	differentSecret := []byte("completely-different-secret-min-32-bytes!")
	_, err = ValidateAccessToken(tokenStr, differentSecret)
	if err == nil {
		t.Fatal("ValidateAccessToken accepted a token signed with a different secret")
	}
}

func TestValidateAccessToken_TamperedPayloadIsRejected(t *testing.T) {
	userID := uuid.New()
	tokenStr, err := GenerateAccessToken(userID, uuid.New(), testSecret)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	// A JWT has the form header.payload.signature.
	// Replace the payload segment to simulate tampering.
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected JWT structure: %s", tokenStr)
	}

	// Build a different (but valid-base64url) payload.
	parts[1] = "dGFtcGVyZWQ" // base64url("tampered")
	tampered := strings.Join(parts, ".")

	_, err = ValidateAccessToken(tampered, testSecret)
	if err == nil {
		t.Fatal("ValidateAccessToken accepted a token with a tampered payload")
	}
}

func TestGenerateAccessToken_ClaimsContent(t *testing.T) {
	userID := uuid.New()
	sessionID := uuid.New()
	before := time.Now().UTC()
	tokenStr, err := GenerateAccessToken(userID, sessionID, testSecret)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	claims, err := ValidateAccessToken(tokenStr, testSecret)
	if err != nil {
		t.Fatalf("ValidateAccessToken error: %v", err)
	}

	// sub claim must equal the user UUID string.
	if claims.Subject != userID.String() {
		t.Errorf("claims.Subject = %q, want %q", claims.Subject, userID.String())
	}

	// jti must be a parseable UUID v4.
	jtiParsed, parseErr := uuid.Parse(claims.ID)
	if parseErr != nil {
		t.Errorf("claims.ID is not a valid UUID: %v", parseErr)
	}
	if jtiParsed.Version() != 4 {
		t.Errorf("jti UUID version = %d, want 4", jtiParsed.Version())
	}

	// sid claim must equal the provided session UUID.
	if claims.SessionID != sessionID {
		t.Errorf("claims.SessionID = %v, want %v", claims.SessionID, sessionID)
	}

	// exp must be approximately now + 15 minutes.
	// JWT stores Unix timestamps as integer seconds, so we compare with a
	// generous ±2 second window to account for sub-second clock differences
	// between before/after measurements and the token's second-precision exp.
	expTime := claims.ExpiresAt.Time.UTC()
	expectedExpMin := before.Add(15*time.Minute - 2*time.Second)
	expectedExpMax := after.Add(15*time.Minute + 2*time.Second)

	if expTime.Before(expectedExpMin) || expTime.After(expectedExpMax) {
		t.Errorf("exp = %v, want in range [%v, %v]", expTime, expectedExpMin, expectedExpMax)
	}
}

// ---------------------------------------------------------------------------
// GenerateRefreshToken
// ---------------------------------------------------------------------------

func TestGenerateRefreshToken_ReturnsNonEmptyValues(t *testing.T) {
	raw, hash, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken returned unexpected error: %v", err)
	}
	if raw == "" {
		t.Error("raw refresh token is empty")
	}
	if hash == "" {
		t.Error("hash is empty")
	}
}

func TestGenerateRefreshToken_HashIsSHA256OfRaw(t *testing.T) {
	raw, hash, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken error: %v", err)
	}

	// raw is hex-encoded 32 bytes; decode back to bytes before hashing.
	rawBytes, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatalf("raw token is not valid hex: %v", err)
	}
	expectedSum := sha256.Sum256(rawBytes)
	expectedHash := hex.EncodeToString(expectedSum[:])

	if hash != expectedHash {
		t.Errorf("hash = %q, want SHA-256 of raw bytes = %q", hash, expectedHash)
	}
}

func TestGenerateRefreshToken_RawTokenIs32Bytes(t *testing.T) {
	raw, _, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken error: %v", err)
	}

	// raw is hex-encoded: 32 bytes = 64 hex characters.
	if len(raw) != 64 {
		t.Errorf("raw hex-encoded token length = %d, want 64 (32 bytes hex-encoded)", len(raw))
	}

	// Confirm it decodes cleanly to exactly 32 bytes.
	decoded, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatalf("raw token is not valid hex: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("decoded raw token length = %d bytes, want 32", len(decoded))
	}
}

func TestGenerateRefreshToken_TwoCallsReturnDifferentTokens(t *testing.T) {
	raw1, _, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("first GenerateRefreshToken error: %v", err)
	}
	raw2, _, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("second GenerateRefreshToken error: %v", err)
	}
	if raw1 == raw2 {
		t.Error("two consecutive calls returned the same raw token (insufficient randomness)")
	}
}

func TestGenerateRefreshToken_RawDoesNotEqualHash(t *testing.T) {
	raw, hash, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken error: %v", err)
	}
	if raw == hash {
		t.Error("raw token equals its own hash — hashing is not being applied")
	}
}
