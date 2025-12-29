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
	"www.theskyscape.com/internal/oauth"
	"www.theskyscape.com/models"
)

func OAuth() (string, *OAuthController) {
	return "oauth", &OAuthController{}
}

type OAuthController struct {
	application.Controller
}

// OAuthClient interface for both App and Project OAuth support
type OAuthClient interface {
	GetID() string
	GetName() string
	RedirectURI() string
	AllowedScopes() string
	VerifySecret(secret string) bool
	IsProject() bool
}

// appClient wraps App to implement OAuthClient
type appClient struct{ *models.App }

func (a appClient) GetID() string   { return a.ID }
func (a appClient) GetName() string { return a.Name }
func (a appClient) IsProject() bool { return false }

// projectClient wraps Project to implement OAuthClient
type projectClient struct{ *models.Project }

func (p projectClient) GetID() string   { return p.ID }
func (p projectClient) GetName() string { return p.Name }
func (p projectClient) IsProject() bool { return true }

// getOAuthClient looks up an OAuth client by ID, checking both apps and projects
func getOAuthClient(clientID string) (OAuthClient, error) {
	// Try app first
	if app, err := models.Apps.Get(clientID); err == nil {
		return appClient{app}, nil
	}
	// Try project
	if project, err := models.Projects.Get(clientID); err == nil {
		return projectClient{project}, nil
	}
	return nil, errors.New("client not found")
}

func (c *OAuthController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := c.Use("auth").(*AuthController)

	// Authorization flow - use Controller.Required (auth only, no profile check)
	http.Handle("GET /oauth/authorize", c.ProtectFunc(c.authorizeGet, auth.Required))
	http.Handle("POST /oauth/authorize", c.ProtectFunc(c.authorize, auth.Required))
	// Token endpoint uses Basic Auth, no CSRF protection needed (server-to-server)
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

// CurrentApp returns the app for the current OAuth request (client_id = app_id)
func (c *OAuthController) CurrentApp() *models.App {
	clientID := c.URL.Query().Get("client_id")
	if clientID == "" {
		return nil
	}
	app, _ := models.Apps.Get(clientID)
	return app
}

// CurrentProject returns the project for the current OAuth request (client_id = project_id)
func (c *OAuthController) CurrentProject() *models.Project {
	clientID := c.URL.Query().Get("client_id")
	if clientID == "" {
		return nil
	}
	project, _ := models.Projects.Get(clientID)
	return project
}

// CurrentClient returns the OAuth client (app or project) for the request
func (c *OAuthController) CurrentClient() OAuthClient {
	clientID := c.URL.Query().Get("client_id")
	if clientID == "" {
		return nil
	}
	client, _ := getOAuthClient(clientID)
	return client
}

// RequestedScopes returns the scopes being requested
func (c *OAuthController) RequestedScopes() []string {
	scope := c.URL.Query().Get("scope")
	if scope == "" {
		scope = "user:read"
	}
	return strings.Split(scope, " ")
}

// ScopesMatch checks if requested scopes match existing authorization
func (c *OAuthController) ScopesMatch() bool {
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(c.Request)
	if user == nil {
		return false
	}

	clientID := c.URL.Query().Get("client_id")
	if clientID == "" {
		return false
	}

	// Check both AppID and ProjectID
	existing, err := models.OAuthAuthorizations.First(
		"WHERE UserID = ? AND (AppID = ? OR ProjectID = ?) AND Revoked = false",
		user.ID, clientID, clientID,
	)
	if err != nil {
		return false
	}

	scope := c.URL.Query().Get("scope")
	if scope == "" {
		scope = "user:read"
	}

	return existing.Scopes == scope
}

// authorizeGet handles the authorization consent screen (or auto-redirects if already authorized)
func (c *OAuthController) authorizeGet(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Parse parameters
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	responseType := r.URL.Query().Get("response_type")
	scope := r.URL.Query().Get("scope")
	state := r.URL.Query().Get("state")

	if clientID == "" || redirectURI == "" {
		http.Error(w, "Missing client_id or redirect_uri", http.StatusBadRequest)
		return
	}

	if responseType != "code" {
		http.Error(w, "response_type must be 'code'", http.StatusBadRequest)
		return
	}

	if scope == "" {
		scope = "user:read"
	}

	// Get and validate client (app or project)
	client, err := getOAuthClient(clientID)
	if err != nil {
		http.Error(w, "Invalid client_id", http.StatusBadRequest)
		return
	}

	// Validate redirect URI matches opinionated format
	if redirectURI != client.RedirectURI() {
		http.Error(w, "Invalid redirect_uri", http.StatusBadRequest)
		return
	}

	// Check if user has already authorized this client with the same scopes
	existing, err := models.OAuthAuthorizations.First(
		"WHERE UserID = ? AND (AppID = ? OR ProjectID = ?) AND Revoked = false",
		user.ID, clientID, clientID,
	)

	// If already authorized with same scopes, skip consent screen
	if err == nil && existing != nil && existing.Scopes == scope {
		// Generate authorization code
		code, err := oauth.CreateAuthorizationCode(clientID, user.ID, redirectURI, scope)
		if err != nil {
			http.Error(w, "Failed to generate authorization code", http.StatusInternalServerError)
			return
		}

		// Redirect back to client with code
		redirectURL := fmt.Sprintf("%s?code=%s&state=%s", redirectURI, url.QueryEscape(code), url.QueryEscape(state))
		c.Redirect(w, r, redirectURL)
		return
	}

	// Show consent screen
	c.Render(w, r, "authorize.html", nil)
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
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	responseType := r.URL.Query().Get("response_type")
	scope := r.URL.Query().Get("scope")
	state := r.URL.Query().Get("state")

	if clientID == "" || redirectURI == "" {
		http.Error(w, "Missing client_id or redirect_uri", http.StatusBadRequest)
		return
	}

	if responseType != "code" {
		http.Error(w, "response_type must be 'code'", http.StatusBadRequest)
		return
	}

	if scope == "" {
		scope = "user:read"
	}

	// Get and validate client (app or project)
	client, err := getOAuthClient(clientID)
	if err != nil {
		http.Error(w, "Invalid client_id", http.StatusBadRequest)
		return
	}

	// Validate redirect URI matches opinionated format
	if redirectURI != client.RedirectURI() {
		http.Error(w, "Invalid redirect_uri", http.StatusBadRequest)
		return
	}

	// Check if user denied
	if r.FormValue("action") == "deny" {
		redirectURL := fmt.Sprintf("%s?error=access_denied&state=%s", redirectURI, url.QueryEscape(state))
		c.Redirect(w, r, redirectURL)
		return
	}

	// Create or update authorization
	authorization, isNew, err := oauth.CreateOrUpdateAuthorizationForClient(user.ID, clientID, scope, client.IsProject())
	if err != nil {
		http.Error(w, "Failed to create authorization", http.StatusInternalServerError)
		return
	}

	// Create activity for first-time authorization
	if isNew {
		subjectType := "app"
		subjectID := authorization.AppID
		if client.IsProject() {
			subjectType = "project"
			subjectID = authorization.ProjectID
		}
		models.Activities.Insert(&models.Activity{
			UserID:      user.ID,
			Action:      "joined",
			SubjectType: subjectType,
			SubjectID:   subjectID,
		})
	}

	// Generate authorization code
	code, err := oauth.CreateAuthorizationCode(clientID, user.ID, redirectURI, scope)
	if err != nil {
		http.Error(w, "Failed to generate authorization code", http.StatusInternalServerError)
		return
	}

	// Redirect back to client with code
	redirectURL := fmt.Sprintf("%s?code=%s&state=%s", redirectURI, url.QueryEscape(code), url.QueryEscape(state))
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

	// Validate client (app or project)
	client, err := getOAuthClient(req.ClientID)
	if err != nil {
		JSONError(w, http.StatusUnauthorized, "Invalid client")
		return
	}

	// Verify client secret
	if !client.VerifySecret(req.ClientSecret) {
		JSONError(w, http.StatusUnauthorized, "Invalid client credentials")
		return
	}

	// Sync database to ensure we have latest state from primary
	models.DB.Sync()

	// Find authorization code
	hashedCode := oauth.HashToken(req.Code)
	authCode, err := models.OAuthAuthorizationCodes.First(
		"WHERE ClientID = ? AND Code = ?",
		req.ClientID, hashedCode,
	)
	if err != nil || authCode == nil {
		JSONError(w, http.StatusBadRequest, "Authorization code not found")
		return
	}

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
	secret, err := oauth.GenerateToken(32)
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
		"WHERE AppID = ? AND UserID = ?",
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
