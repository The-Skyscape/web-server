package models

import (
	"crypto/sha256"
	"encoding/base64"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
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
	computed := base64.StdEncoding.EncodeToString(hash[:])
	return computed == c.Code
}
