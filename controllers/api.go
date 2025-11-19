package controllers

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/golang-jwt/jwt/v5"
	"www.theskyscape.com/models"
)

func API() (string, *APIController) {
	return "api", &APIController{}
}

type APIController struct {
	application.Controller
}

func (c *APIController) Setup(app *application.App) {
	c.Controller.Setup(app)

	http.Handle("GET /api/user", http.HandlerFunc(c.getUser))
}

func (c APIController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

// ValidateAccessToken extracts and validates the Bearer token from the request
// Returns the user, scopes, and any error
func (c *APIController) ValidateAccessToken(r *http.Request) (*authentication.User, []string, error) {
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

	clientID, ok := claims["client_id"].(string)
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
		"WHERE UserID = ? AND ClientID = ? AND RevokedAt IS NULL",
		userID, clientID,
	)

	if err != nil || auth == nil {
		return nil, nil, errors.New("authorization has been revoked")
	}

	// Get user
	user, err := models.Auth.Users.Get(userID)
	if err != nil {
		return nil, nil, errors.New("user not found")
	}

	return user, scopes, nil
}

// requireScope checks if the provided scopes contain the required scope
func requireScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == required {
			return true
		}
	}
	return false
}

// UserResponse holds the user API response
type UserResponse struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Avatar string `json:"avatar"`
}

// getUser returns the authenticated user's information
func (c *APIController) getUser(w http.ResponseWriter, r *http.Request) {
	// Validate access token
	user, scopes, err := c.ValidateAccessToken(r)
	if err != nil {
		JSONError(w, http.StatusUnauthorized, err.Error())
		return
	}

	// Check required scope
	if !requireScope(scopes, "user:read") {
		JSONError(w, http.StatusForbidden, "insufficient scope")
		return
	}

	// Return user information
	response := &UserResponse{
		ID:     user.ID,
		Handle: user.Handle,
		Name:   user.Name,
		Email:  user.Email,
		Avatar: user.Avatar,
	}

	JSONSuccess(w, response)
}
