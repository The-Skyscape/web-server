package models

import (
	"fmt"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"golang.org/x/crypto/bcrypt"
)

type App struct {
	application.Model
	RepoID            string
	Name              string
	Description       string
	Status            string
	Error             string
	OAuthClientSecret string // bcrypt hashed
	DatabaseEnabled   bool   // Whether app has database provisioned
}

func (*App) Table() string { return "apps" }

// NewApp creates a new app record. Caller is responsible for:
// - Sanitizing the ID (use hosting.SanitizeID)
// - Creating the activity
// - Triggering the build (use hosting.BuildApp)
func NewApp(id, repoID, name, description string, databaseEnabled bool) (*App, error) {
	app := &App{
		Model:           application.Model{ID: id},
		RepoID:          repoID,
		Name:            name,
		Description:     description,
		DatabaseEnabled: databaseEnabled,
	}
	return Apps.Insert(app)
}

func (a *App) Repo() *Repo {
	repo, err := Repos.Get(a.RepoID)
	if err != nil {
		return nil
	}

	return repo
}

func (a *App) Owner() *authentication.User {
	repo := a.Repo()
	if repo == nil {
		return nil
	}

	return repo.Owner()
}

func (a *App) RedirectURI() string {
	return fmt.Sprintf("https://%s.skysca.pe/auth/callback", a.ID)
}

func (a *App) AllowedScopes() string {
	return "user:read"
}

func (a *App) VerifySecret(secret string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(a.OAuthClientSecret), []byte(secret))
	return err == nil
}

// ActivePromotion returns the current active (non-expired) promotion for this app, if any
func (a *App) ActivePromotion() *Promotion {
	promo, _ := Promotions.First(`
		WHERE SubjectType = 'app' AND SubjectID = ? AND ExpiresAt > ?
		ORDER BY CreatedAt DESC
	`, a.ID, time.Now())
	return promo
}

// ActiveImage returns the current running image for this app, if any
func (a *App) ActiveImage() *Image {
	img, _ := Images.First(`
		WHERE AppID = ? AND Status = 'running'
		ORDER BY CreatedAt DESC
	`, a.ID)
	return img
}

func (app *App) Images() []*Image {
	images, err := Images.Search(`
		WHERE AppID = ?
		ORDER BY CreatedAt DESC
	`, app.ID)
	if err != nil {
		return nil
	}

	return images
}

func (a *App) Comments(limit, offset int) []*Comment {
	comments, _ := Comments.Search(`
		WHERE SubjectID = ?
			AND Content != ''
		ORDER BY CreatedAt DESC
		LIMIT ? OFFSET ?
	`, a.ID, limit, offset)
	return comments
}

// AuthorizedUsersCount returns the number of users who have authorized this app
func (a *App) AuthorizedUsersCount() int {
	return OAuthAuthorizations.Count("WHERE AppID = ? AND Revoked = false", a.ID)
}

