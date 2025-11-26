package session

import (
	"log/slog"
	"os"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNewJWTManager(t *testing.T) {
	t.Run("with custom key", func(t *testing.T) {
		key := "test-signing-key"
		manager, err := NewJWTManager(key, 0, testLogger(), nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if manager == nil {
			t.Fatal("expected manager to be created")
		}
		if string(manager.signingKey) != key {
			t.Errorf("expected signing key %s, got %s", key, string(manager.signingKey))
		}
		if manager.duration != DefaultSessionDuration {
			t.Errorf("expected duration %v, got %v", DefaultSessionDuration, manager.duration)
		}
	})

	t.Run("with custom session duration", func(t *testing.T) {
		key := "test-signing-key"
		manager, err := NewJWTManager(key, 48, testLogger(), nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedDuration := 48 * time.Minute
		if manager.duration != expectedDuration {
			t.Errorf("expected duration %v, got %v", expectedDuration, manager.duration)
		}
	})

	t.Run("with empty key returns error", func(t *testing.T) {
		manager, err := NewJWTManager("", 0, testLogger(), nil)

		if err == nil {
			t.Error("expected error for empty signing key")
		}
		if manager != nil {
			t.Error("expected nil manager for empty key")
		}
	})
}

func TestGenerate(t *testing.T) {
	manager, _ := NewJWTManager("test-key", 0, testLogger(), nil)

	t.Run("generates valid JWT", func(t *testing.T) {
		token := manager.Generate()

		if token == "" {
			t.Error("expected non-empty token")
		}

		// validate the token can be parsed
		isNotAllowed, err := manager.Validate(token)
		if err != nil {
			t.Fatalf("failed to validate token: %v", err)
		}
		if isNotAllowed {
			t.Error("expected token to be allowed")
		}
	})

	t.Run("generates tokens that can be validated", func(t *testing.T) {
		token := manager.Generate()

		// parse and check claims directly
		parsedToken, err := jwt.ParseWithClaims(token, &Claims{}, func(_ *jwt.Token) (interface{}, error) {
			return manager.signingKey, nil
		})
		if err != nil {
			t.Fatalf("failed to parse token: %v", err)
		}

		claims, ok := parsedToken.Claims.(*Claims)
		if !ok {
			t.Fatal("failed to extract claims")
		}

		if claims.Issuer != "mcp-gateway" {
			t.Errorf("expected issuer 'mcp-gateway', got %s", claims.Issuer)
		}
		if claims.IssuedAt == nil {
			t.Error("expected issued at timestamp")
		}
		if claims.ExpiresAt == nil {
			t.Error("expected expiration timestamp")
		}
		if len(claims.Audience) == 0 || claims.Audience[0] != "mcp-gateway" {
			t.Errorf("expected audience 'mcp-gateway', got %v", claims.Audience)
		}
	})
}

func TestValidate(t *testing.T) {
	manager, _ := NewJWTManager("test-key", 0, testLogger(), nil)

	t.Run("validates correct token", func(t *testing.T) {
		token := manager.Generate()

		isNotAllowed, err := manager.Validate(token)
		if err != nil {
			t.Fatalf("failed to validate valid token: %v", err)
		}
		if isNotAllowed {
			t.Error("expected token to be allowed (isNotAllowed should be false)")
		}
	})

	t.Run("rejects token with wrong signing key", func(t *testing.T) {
		otherManager, _ := NewJWTManager("different-key", 0, testLogger(), nil)
		token := otherManager.Generate()

		isNotAllowed, err := manager.Validate(token)
		if err == nil {
			t.Error("expected error for token signed with different key")
		}
		if !isNotAllowed {
			t.Error("expected isNotAllowed to be true for invalid token")
		}
	})

	t.Run("rejects invalid token format", func(t *testing.T) {
		isNotAllowed, err := manager.Validate("not-a-jwt-token")
		if err == nil {
			t.Error("expected error for invalid token format")
		}
		if !isNotAllowed {
			t.Error("expected isNotAllowed to be true for malformed token")
		}
	})

	t.Run("rejects expired token", func(t *testing.T) {
		// create a manager with very short duration
		shortManager, _ := NewJWTManager("test-key", 0, testLogger(), nil)
		shortManager.duration = 1 * time.Nanosecond

		token := shortManager.Generate()
		time.Sleep(10 * time.Millisecond)

		isNotAllowed, err := manager.Validate(token)
		if err == nil {
			t.Error("expected error for expired token")
		}
		if !isNotAllowed {
			t.Error("expected isNotAllowed to be true for expired token")
		}
	})

	t.Run("rejects token with wrong algorithm", func(t *testing.T) {
		// create token with None algorithm instead of HS256
		claims := Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: "mcp-gateway",
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
		tokenString, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

		isNotAllowed, err := manager.Validate(tokenString)
		if err == nil {
			t.Error("expected error for wrong signing algorithm")
		}
		if !isNotAllowed {
			t.Error("expected isNotAllowed to be true for wrong algorithm")
		}
	})
}

func TestTerminate(t *testing.T) {
	manager, _ := NewJWTManager("test-key", 0, testLogger(), nil)

	t.Run("terminate returns no error", func(t *testing.T) {
		token := manager.Generate()

		isNotAllowed, err := manager.Terminate(token)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if isNotAllowed {
			t.Error("expected isNotAllowed to be false")
		}
	})
}
