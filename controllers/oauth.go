package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"www.theskyscape.com/models"
)

func OAuth() (string, *OAuthController) {
	return "oauth", &OAuthController{}
}

type OAuthController struct {
	application.Controller
}

func (c *OAuthController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := c.Use("auth").(*AuthController)

	// Authorization flow
	http.Handle("GET /oauth/authorize", c.Serve("oauth/authorize.html", auth.Required))
	http.Handle("POST /oauth/authorize", c.ProtectFunc(c.authorize, auth.Required))
	http.Handle("POST /oauth/token", http.HandlerFunc(c.token))

	// OAuth client management for apps
	http.Handle("GET /app/{app}/users", c.Serve("app-users.html", auth.Required))
	http.Handle("POST /app/{app}/oauth/regenerate", c.ProtectFunc(c.regenerateSecret, auth.Required))
	http.Handle("DELETE /app/{app}/users/{user}", c.ProtectFunc(c.revokeUser, auth.Required))
}

func (c OAuthController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

// AuthorizeParams holds the authorization request parameters
type AuthorizeParams struct {
	ClientID     string
	RedirectURI  string
	ResponseType string
	Scope        string
	State        string
}

// ParseAuthorizeParams parses and validates authorization request parameters
func (c *OAuthController) ParseAuthorizeParams() (*AuthorizeParams, error) {
	params := &AuthorizeParams{
		ClientID:     c.URL.Query().Get("client_id"),
		RedirectURI:  c.URL.Query().Get("redirect_uri"),
		ResponseType: c.URL.Query().Get("response_type"),
		Scope:        c.URL.Query().Get("scope"),
		State:        c.URL.Query().Get("state"),
	}

	if params.ClientID == "" {
		return nil, errors.New("client_id is required")
	}

	if params.RedirectURI == "" {
		return nil, errors.New("redirect_uri is required")
	}

	if params.ResponseType != "code" {
		return nil, errors.New("response_type must be 'code'")
	}

	if params.Scope == "" {
		params.Scope = "user:read" // Default scope
	}

	return params, nil
}

// CurrentApp returns the app for the current OAuth request (client_id = app_id)
func (c *OAuthController) CurrentApp() *models.App {
	params, err := c.ParseAuthorizeParams()
	if err != nil {
		return nil
	}

	app, err := models.Apps.Get(params.ClientID)
	if err != nil {
		return nil
	}

	return app
}

// ExistingAuthorization returns the user's existing authorization for this client
func (c *OAuthController) ExistingAuthorization() *models.OAuthAuthorization {
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(c.Request)
	if user == nil {
		return nil
	}

	params, err := c.ParseAuthorizeParams()
	if err != nil {
		return nil
	}

	existing, err := models.OAuthAuthorizations.First(
		"WHERE UserID = ? AND ClientID = ? AND RevokedAt IS NULL",
		user.ID, params.ClientID,
	)
	if err != nil {
		return nil
	}

	return existing
}

// RequestedScopes returns the scopes being requested
func (c *OAuthController) RequestedScopes() []string {
	params, _ := c.ParseAuthorizeParams()
	if params == nil {
		return []string{}
	}

	return strings.Split(params.Scope, " ")
}

// ScopesMatch checks if requested scopes match existing authorization
func (c *OAuthController) ScopesMatch() bool {
	existing := c.ExistingAuthorization()
	if existing == nil {
		return false
	}

	params, _ := c.ParseAuthorizeParams()
	if params == nil {
		return false
	}

	return existing.Scopes == params.Scope
}

// authorize handles the authorization consent submission
func (c *OAuthController) authorize(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Parse parameters
	params, err := c.ParseAuthorizeParams()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get and validate app (client_id = app_id)
	app, err := models.Apps.Get(params.ClientID)
	if err != nil {
		http.Error(w, "Invalid client_id", http.StatusBadRequest)
		return
	}

	// Validate redirect URI matches opinionated format
	expectedRedirectURI := app.RedirectURI()
	if params.RedirectURI != expectedRedirectURI {
		http.Error(w, "Invalid redirect_uri", http.StatusBadRequest)
		return
	}

	// Check if user denied
	if r.FormValue("action") == "deny" {
		redirectURL := fmt.Sprintf("%s?error=access_denied&state=%s",
			params.RedirectURI, url.QueryEscape(params.State))
		c.Redirect(w, r, redirectURL)
		return
	}

	// Create or update authorization
	if _, err := models.CreateOrUpdateAuthorization(user.ID, params.ClientID, params.Scope); err != nil {
		http.Error(w, "Failed to create authorization", http.StatusInternalServerError)
		return
	}

	// Generate authorization code
	code, err := models.CreateAuthorizationCode(params.ClientID, user.ID, params.RedirectURI, params.Scope)
	if err != nil {
		http.Error(w, "Failed to generate authorization code", http.StatusInternalServerError)
		return
	}

	// Redirect back to client with code
	redirectURL := fmt.Sprintf("%s?code=%s&state=%s",
		params.RedirectURI, url.QueryEscape(code), url.QueryEscape(params.State))
	c.Redirect(w, r, redirectURL)
}

// TokenRequest holds the token exchange request parameters
type TokenRequest struct {
	GrantType    string
	Code         string
	RedirectURI  string
	ClientID     string
	ClientSecret string
}

// TokenResponse holds the token response
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

// token handles the token exchange endpoint
func (c *OAuthController) token(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	if err := r.ParseForm(); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	// Extract client credentials from Basic Auth
	clientID, clientSecret, ok := r.BasicAuth()
	if !ok {
		JSONError(w, http.StatusUnauthorized, "Client authentication required")
		return
	}

	// Parse request
	req := &TokenRequest{
		GrantType:    r.FormValue("grant_type"),
		Code:         r.FormValue("code"),
		RedirectURI:  r.FormValue("redirect_uri"),
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}

	// Validate grant type
	if req.GrantType != "authorization_code" {
		JSONError(w, http.StatusBadRequest, "Unsupported grant_type")
		return
	}

	// Validate client (client_id = app_id)
	app, err := models.Apps.Get(req.ClientID)
	if err != nil {
		JSONError(w, http.StatusUnauthorized, "Invalid client")
		return
	}

	// Verify client secret
	if !app.VerifySecret(req.ClientSecret) {
		JSONError(w, http.StatusUnauthorized, "Invalid client credentials")
		return
	}

	// Find authorization code
	hashedCode := models.HashToken(req.Code)
	authCodes, err := models.OAuthAuthorizationCodes.Search(
		"WHERE ClientID = ? AND Code = ? ORDER BY CreatedAt DESC LIMIT 1",
		req.ClientID, hashedCode,
	)

	if err != nil || len(authCodes) == 0 {
		JSONError(w, http.StatusBadRequest, "Invalid authorization code")
		return
	}

	authCode := authCodes[0]

	// Validate authorization code
	if !authCode.IsValid() {
		JSONError(w, http.StatusBadRequest, "Authorization code expired or already used")
		return
	}

	// Validate redirect URI matches
	if authCode.RedirectURI != req.RedirectURI {
		JSONError(w, http.StatusBadRequest, "Redirect URI mismatch")
		return
	}

	// Mark code as used
	if err := authCode.MarkAsUsed(); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to process authorization code")
		return
	}

	// Generate JWT access token
	accessToken, err := c.generateAccessToken(authCode.UserID, authCode.ClientID, authCode.Scopes)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to generate access token")
		return
	}

	// Return token response
	response := &TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   30 * 24 * 60 * 60, // 30 days in seconds
		Scope:       authCode.Scopes,
	}

	JSONSuccess(w, response)
}

// generateAccessToken creates a signed JWT access token
func (c *OAuthController) generateAccessToken(userID, clientID, scopes string) (string, error) {
	secret := os.Getenv("AUTH_SECRET")
	if secret == "" {
		return "", errors.New("AUTH_SECRET not configured")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"sub":       userID,
		"client_id": clientID,
		"scope":     scopes,
		"iat":       now.Unix(),
		"exp":       now.Add(30 * 24 * time.Hour).Unix(), // 30 days
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// AuthorizedUsers returns the list of users who have authorized the current app
func (c *OAuthController) AuthorizedUsers() []*models.OAuthAuthorization {
	appID := c.PathValue("app")
	app, err := models.Apps.Get(appID)
	if err != nil || app == nil {
		return nil
	}

	// Get all non-revoked authorizations (ClientID = AppID)
	auths, _ := models.OAuthAuthorizations.Search(
		"WHERE ClientID = ? AND RevokedAt IS NULL ORDER BY CreatedAt DESC",
		app.ID,
	)

	return auths
}

// regenerateSecret regenerates the OAuth client secret
func (c *OAuthController) regenerateSecret(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	app, err := models.Apps.Get(r.PathValue("app"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("app not found"))
		return
	}

	repo := app.Repo()
	if repo == nil || repo.OwnerID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("permission denied"))
		return
	}

	// Generate new secret
	secret, err := models.GenerateRandomToken(32)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Hash and update app
	hashedSecret, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	app.OAuthClientSecret = string(hashedSecret)
	if err := models.Apps.Update(app); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Return success with secret
	w.Header().Set("HX-Trigger", `{"showSecret": {"secret": "`+secret+`"}}`)
	c.Refresh(w, r)
}

// revokeUser revokes a specific user's authorization
func (c *OAuthController) revokeUser(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	app, err := models.Apps.Get(r.PathValue("app"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("app not found"))
		return
	}

	repo := app.Repo()
	if repo == nil || repo.OwnerID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("permission denied"))
		return
	}

	userID := r.PathValue("user")

	// Find and revoke authorization
	authorization, err := models.OAuthAuthorizations.First(
		"WHERE ClientID = ? AND UserID = ?",
		app.ID, userID,
	)

	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authorization not found"))
		return
	}

	if err := authorization.Revoke(); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}
