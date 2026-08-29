package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
)

// writeJSONError writes a JSON error response with the given message and status code
func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// AuthConfig holds the configuration for the auth middleware
type AuthConfig struct {
	GetJWTSecret func() ([]byte, error)
	ErrorWriter  func(http.ResponseWriter, *http.Request, string, int)
}

// NewAuthMiddleware creates a new auth middleware with the given JWT secret provider
func NewAuthMiddleware(getJWTSecret func() ([]byte, error)) *AuthConfig {
	return &AuthConfig{
		GetJWTSecret: getJWTSecret,
	}
}

func (ac *AuthConfig) writeError(w http.ResponseWriter, r *http.Request, message string, statusCode int) {
	if ac.ErrorWriter != nil {
		ac.ErrorWriter(w, r, message, statusCode)
		return
	}
	writeJSONError(w, message, statusCode)
}

// RequireAuth creates a middleware that verifies the JWT token and user session
func (ac *AuthConfig) RequireAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				ac.writeError(w, r, "Authorization header is required", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				ac.writeError(w, r, "Invalid authorization header format", http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]

			// Get JWT secret
			secret, err := ac.GetJWTSecret()
			if err != nil {
				ac.writeError(w, r, "Authentication unavailable", http.StatusServiceUnavailable)
				return
			}

			// Parse and validate token with claims
			claims := &service.UserClaims{}
			token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
				// CRITICAL: Verify signing method to prevent algorithm confusion
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return secret, nil
			})

			// CRITICAL: Check both error AND token.Valid
			if err != nil {
				ac.writeError(w, r, fmt.Sprintf("Invalid token: %v", err), http.StatusUnauthorized)
				return
			}
			if !token.Valid {
				ac.writeError(w, r, "Invalid token", http.StatusUnauthorized)
				return
			}

			// Validate required claims
			if claims.UserID == "" {
				ac.writeError(w, r, "User ID not found in token", http.StatusUnauthorized)
				return
			}
			if claims.Type == "" {
				ac.writeError(w, r, "User type not found in token", http.StatusUnauthorized)
				return
			}
			if claims.Type == string(domain.UserTypeUser) && claims.SessionID == "" {
				ac.writeError(w, r, "Session ID not found in token", http.StatusUnauthorized)
				return
			}

			// Set context values
			ctx := context.WithValue(r.Context(), domain.UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, domain.UserTypeKey, claims.Type)
			if claims.Type == string(domain.UserTypeUser) {
				ctx = context.WithValue(ctx, domain.SessionIDKey, claims.SessionID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RestrictedInDemo creates a middleware that returns a 400 error if the server is in demo mode
//
// Usage example:
//
//	restrictedMiddleware := middleware.RestrictedInDemo(true)
//	mux.Handle("/api/sensitive.operation", restrictedMiddleware(http.HandlerFunc(handler)))
//
// This middleware should be applied to endpoints that should be disabled in demo environments,
// such as operations that modify critical data or perform destructive actions.
func RestrictedInDemo(isDemo bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isDemo {
				writeJSONError(w, "This operation is not allowed in demo mode", http.StatusBadRequest)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
