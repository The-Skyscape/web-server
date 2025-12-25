package security

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/golang-jwt/jwt/v5"
	"www.theskyscape.com/models"
)

type contextKey string

const (
	userContextKey   contextKey = "api_user"
	scopesContextKey contextKey = "api_scopes"
)

// UserFromContext retrieves the authenticated user from request context
func UserFromContext(r *http.Request) *authentication.User {
	if user, ok := r.Context().Value(userContextKey).(*authentication.User); ok {
		return user
	}
	return nil
}

// ScopesFromContext retrieves the scopes from request context
func ScopesFromContext(r *http.Request) []string {
	if scopes, ok := r.Context().Value(scopesContextKey).([]string); ok {
		return scopes
	}
	return nil
}

func ParseAccessToken(r *http.Request) (*authentication.User, []string, error) {
	// Extract Bearer token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, nil, errors.New("missing authorization header")
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, nil, errors.New("invalid authorization header format")
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	// Parse and validate JWT
	secret := os.Getenv("AUTH_SECRET")
	if secret == "" {
		return nil, nil, errors.New("server configuration error")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, nil, errors.New("invalid token")
	}

	if !token.Valid {
		return nil, nil, errors.New("token is not valid")
	}

	// Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, nil, errors.New("invalid token claims")
	}

	userID, ok := claims["sub"].(string)
	if !ok {
		return nil, nil, errors.New("missing user ID in token")
	}

	appID, ok := claims["client_id"].(string)
	if !ok {
		return nil, nil, errors.New("missing client ID in token")
	}

	scopeStr, ok := claims["scope"].(string)
	if !ok {
		return nil, nil, errors.New("missing scopes in token")
	}

	scopes := strings.Split(scopeStr, " ")

	// Check if authorization still exists and is not revoked
	auth, err := models.OAuthAuthorizations.First(
		"WHERE UserID = ? AND AppID = ? AND Revoked = false",
		userID, appID,
	)

	if err != nil || auth == nil {
		return nil, nil, errors.New("authorization not found")
	}

	// Get user
	user, err := models.Auth.Users.Get(userID)
	if err != nil {
		return nil, nil, errors.New("user not found")
	}

	return user, scopes, nil
}

func jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func RequireScopes(required ...string) application.AccessCheck {
	return func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		user, scopes, err := ParseAccessToken(r)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, err.Error())
			return false
		}

		for _, scope := range required {
			if !slices.Contains(scopes, scope) {
				jsonError(w, http.StatusForbidden, "insufficient scope: "+scope)
				return false
			}
		}

		// Store user and scopes in context for handlers
		ctx := r.Context()
		ctx = context.WithValue(ctx, userContextKey, user)
		ctx = context.WithValue(ctx, scopesContextKey, scopes)
		*r = *r.WithContext(ctx)

		return true
	}
}
