// Package session provides JWT-based session ID generation and validation
package session

import (
	"fmt"
	"log/slog"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/mark3labs/mcp-go/server"
)

const (
	// DefaultSessionDuration is the default duration for session JWTs
	DefaultSessionDuration = 24 * time.Hour
	issuer                 = "mcp-gateway"
)

var _ server.SessionIdManager = &JWTManager{}

// Claims represents the claims in a session JWT
type Claims struct {
	jwt.RegisteredClaims
}

// JWTManager handles JWT generation and validation for session IDs
type JWTManager struct {
	signingKey []byte
	duration   time.Duration
	logger     *slog.Logger
}

// NewJWTManager creates a new JWT manager with the provided signing key
func NewJWTManager(signingKey string, sessionLength int64, logger *slog.Logger) (*JWTManager, error) {
	if signingKey == "" {
		return nil, fmt.Errorf("no signing key provided")
	}
	var sessionDuration = DefaultSessionDuration
	if sessionLength != 0 {
		sessionDuration = time.Duration(sessionLength) * time.Hour
	}

	return &JWTManager{
		signingKey: []byte(signingKey),
		duration:   sessionDuration,
		logger:     logger,
	}, nil
}

// generateSessionJWT creates a JWT token
func (m *JWTManager) generateSessionJWT() (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.duration)),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{issuer},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.signingKey)
}

// Generate returns a session id JWT to fullfil SessionIdManager interface
func (m *JWTManager) Generate() string {
	m.logger.Info("gerating session id in jwt session manager")
	sessID, err := m.generateSessionJWT()
	if err != nil {
		m.logger.Error("failed to generate session id", "error", err)
		return ""
	}
	return sessID
}

// Validate validates a JWT token and fullfils SessionIdManager interface
func (m *JWTManager) Validate(tokenValue string) (bool, error) {
	token, err := jwt.ParseWithClaims(tokenValue, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		m.logger.Info("validating JWT session")
		// verify signing method
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.signingKey, nil

	})
	if err != nil {
		return true, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return true, fmt.Errorf("token is invalid")
	}
	return false, nil
}

// Terminate part of the SessionIDManager interface
func (m *JWTManager) Terminate(sessionID string) (isNotAllowed bool, err error) {
	m.logger.Info("terminate session id in jwt session manager", "sesssion", sessionID)
	return false, nil
}
