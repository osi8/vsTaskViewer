package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents JWT claims
type Claims struct {
	TaskID string `json:"task_id"`
	jwt.RegisteredClaims
}

// validateJWT validates the JWT token from the request
// expectedAudience: "" or empty string for API tokens, "viewer" for viewer tokens, nil to skip audience validation
func validateJWT(r *http.Request, secret string, expectedAudience *string) (*Claims, error) {
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		return nil, errors.New("missing token parameter")
	}

	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	// Check expiration
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("token expired")
	}

	// Validate audience (AUD claim)
	if expectedAudience != nil {
		// If expectedAudience is set, token must have matching audience
		// Empty string means API token (no audience or empty audience)
		// "viewer" means viewer token (must have audience="viewer")
		if *expectedAudience == "" {
			// API token: should have no audience or empty audience
			if len(claims.Audience) > 0 && claims.Audience[0] != "" {
				return nil, errors.New("token audience mismatch: API token expected")
			}
		} else {
			// Viewer token: must have matching audience
			if len(claims.Audience) == 0 || claims.Audience[0] != *expectedAudience {
				return nil, fmt.Errorf("token audience mismatch: expected %s", *expectedAudience)
			}
		}
	}

	return claims, nil
}

// authMiddleware wraps a handler with JWT authentication
// expectedAudience: "" for API tokens, "viewer" for viewer tokens, nil to skip audience validation
func authMiddleware(handler http.HandlerFunc, secret string, expectedAudience *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := validateJWT(r, secret, expectedAudience)
		if err != nil {
			http.Error(w, fmt.Sprintf("Unauthorized: %v", err), http.StatusUnauthorized)
			return
		}
		handler(w, r)
	}
}

