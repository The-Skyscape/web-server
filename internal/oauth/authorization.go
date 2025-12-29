package oauth

import (
	"time"

	"www.theskyscape.com/models"
)

const CodeExpiry = 10 * time.Minute

// CreateAuthorizationCode creates a new authorization code for the OAuth flow
func CreateAuthorizationCode(clientID, userID, redirectURI, scopes string) (string, error) {
	code, err := GenerateToken(32)
	if err != nil {
		return "", err
	}

	hashedCode := HashToken(code)

	authCode := &models.OAuthAuthorizationCode{
		ClientID:    clientID,
		UserID:      userID,
		Code:        hashedCode,
		RedirectURI: redirectURI,
		Scopes:      scopes,
		ExpiresAt:   time.Now().Add(CodeExpiry),
		Used:        false,
	}

	if _, err := models.OAuthAuthorizationCodes.Insert(authCode); err != nil {
		return "", err
	}

	return code, nil
}

// CreateOrUpdateAuthorization creates or updates an OAuth authorization for an app
func CreateOrUpdateAuthorization(userID, clientID, scopes string) (*models.OAuthAuthorization, bool, error) {
	existing, err := models.OAuthAuthorizations.First("WHERE UserID = ? AND AppID = ?", userID, clientID)
	if err == nil {
		existing.Scopes = scopes
		existing.Revoked = false
		if err := models.OAuthAuthorizations.Update(existing); err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}

	auth := &models.OAuthAuthorization{
		UserID: userID,
		AppID:  clientID,
		Scopes: scopes,
	}

	created, err := models.OAuthAuthorizations.Insert(auth)
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}

// CreateOrUpdateAuthorizationForClient creates or updates authorization for app or project
func CreateOrUpdateAuthorizationForClient(userID, clientID, scopes string, isProject bool) (*models.OAuthAuthorization, bool, error) {
	existing, err := models.OAuthAuthorizations.First(
		"WHERE UserID = ? AND (AppID = ? OR ProjectID = ?)",
		userID, clientID, clientID,
	)
	if err == nil {
		existing.Scopes = scopes
		existing.Revoked = false
		if isProject {
			existing.ProjectID = clientID
		} else {
			existing.AppID = clientID
		}
		if err := models.OAuthAuthorizations.Update(existing); err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}

	auth := &models.OAuthAuthorization{
		UserID: userID,
		Scopes: scopes,
	}
	if isProject {
		auth.ProjectID = clientID
	} else {
		auth.AppID = clientID
	}

	created, err := models.OAuthAuthorizations.Insert(auth)
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}
