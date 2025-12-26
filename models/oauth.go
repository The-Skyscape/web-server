package models

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/pkg/errors"
)

// OAuthAuthorization represents a user's consent to allow an app/project to access their data
type OAuthAuthorization struct {
	application.Model
	UserID    string
	AppID     string // legacy - for App authorizations
	ProjectID string // new - for Project authorizations
	Scopes    string // space-separated granted scopes
	Revoked   bool
}

func (*OAuthAuthorization) Table() string { return "oauth_authorizations" }

// User returns the user who granted this authorization
func (a *OAuthAuthorization) User() *authentication.User {
	user, _ := Auth.Users.Get(a.UserID)
	return user
}

// App returns the app this authorization is for (legacy)
func (a *OAuthAuthorization) App() *App {
	if a.AppID == "" {
		return nil
	}
	app, _ := Apps.Get(a.AppID)
	return app
}

// Project returns the project this authorization is for
func (a *OAuthAuthorization) Project() *Project {
	if a.ProjectID == "" {
		return nil
	}
	project, _ := Projects.Get(a.ProjectID)
	return project
}

// Revoke marks this authorization as revoked
func (a *OAuthAuthorization) Revoke() error {
	a.Revoked = true
	return OAuthAuthorizations.Update(a)
}

// OAuthAuthorizationCode represents a short-lived code used to exchange for an access token
type OAuthAuthorizationCode struct {
	application.Model
	ClientID    string
	UserID      string
	Code        string // SHA-256 hashed
	RedirectURI string
	Scopes      string // space-separated
	ExpiresAt   time.Time
	Used        bool
}

func (*OAuthAuthorizationCode) Table() string { return "oauth_authorization_codes" }

// IsExpired returns true if this code has expired
func (c *OAuthAuthorizationCode) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// IsValid returns true if the code is not used and not expired
func (c *OAuthAuthorizationCode) IsValid() bool {
	return !c.Used && !c.IsExpired()
}

// MarkAsUsed marks this code as used
func (c *OAuthAuthorizationCode) MarkAsUsed() error {
	c.Used = true
	return OAuthAuthorizationCodes.Update(c)
}

// VerifyCode checks if the provided code matches the stored hash
func (c *OAuthAuthorizationCode) VerifyCode(code string) bool {
	hash := sha256.Sum256([]byte(code))
	storedHash := c.Code
	computedHash := base64.StdEncoding.EncodeToString(hash[:])
	return storedHash == computedHash
}

// GenerateRandomToken generates a cryptographically secure random token
func GenerateRandomToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.Wrap(err, "failed to generate random token")
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// HashToken hashes a token using SHA-256
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// CreateAuthorizationCode creates a new authorization code
func CreateAuthorizationCode(clientID, userID, redirectURI, scopes string) (string, error) {
	// Generate code
	code, err := GenerateRandomToken(32)
	if err != nil {
		return "", err
	}

	// Hash the code
	hashedCode := HashToken(code)

	authCode := &OAuthAuthorizationCode{
		ClientID:    clientID,
		UserID:      userID,
		Code:        hashedCode,
		RedirectURI: redirectURI,
		Scopes:      scopes,
		ExpiresAt:   time.Now().Add(10 * time.Minute),
		Used:        false,
	}

	if _, err := OAuthAuthorizationCodes.Insert(authCode); err != nil {
		return "", err
	}

	return code, nil
}

// CreateOrUpdateAuthorization creates a new authorization or updates existing one
// Returns the authorization and a boolean indicating if it was newly created (true) or updated (false)
func CreateOrUpdateAuthorization(userID, clientID, scopes string) (*OAuthAuthorization, bool, error) {
	// Check if authorization already exists
	existing, err := OAuthAuthorizations.First("WHERE UserID = ? AND AppID = ?", userID, clientID)
	if err == nil {
		// Update existing authorization
		existing.Scopes = scopes
		existing.Revoked = false // Un-revoke if it was revoked
		if err := OAuthAuthorizations.Update(existing); err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}

	// Create new authorization
	auth := &OAuthAuthorization{
		UserID: userID,
		AppID:  clientID,
		Scopes: scopes,
	}

	created, err := OAuthAuthorizations.Insert(auth)
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}
