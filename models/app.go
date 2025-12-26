package models

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/pkg/errors"
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

func NewApp(repo *Repo, name, description string, databaseEnabled bool) (*App, error) {
	// Generate ID from name, sanitizing to only allow safe characters
	id := strings.ToLower(strings.ReplaceAll(name, " ", "-"))

	// Remove any characters that aren't alphanumeric, hyphens, or underscores
	// This prevents command injection and path traversal attacks
	id = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(id, "")

	// Ensure ID isn't empty after sanitization
	if id == "" {
		return nil, errors.New("app name must contain at least one alphanumeric character")
	}

	// Check if an app with this ID already exists
	if _, err := Apps.Get(id); err == nil {
		return nil, errors.New("an app with this ID already exists")
	}

	// OAuth client secret will be generated on first deployment
	app := &App{
		Model:             application.Model{ID: id},
		Name:              name,
		Description:       description,
		RepoID:            repo.ID,
		OAuthClientSecret: "", // Will be set during deployment
		DatabaseEnabled:   databaseEnabled,
	}

	if _, err := Apps.Insert(app); err != nil {
		return nil, err
	}

	Activities.Insert(&Activity{
		UserID:      repo.OwnerID,
		Action:      "launched",
		SubjectType: "app",
		SubjectID:   app.ID,
	})

	// Automatically start building and deploying the app
	go func() {
		app.Status = "launching"
		Apps.Update(app)

		if _, err := app.Build(); err != nil {
			app.Error = err.Error()
			Apps.Update(app)
		}
	}()

	return app, nil
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

func (app *App) Build() (*Image, error) {
	host := containers.Local()
	tmpDir, err := os.MkdirTemp("", "app-*")
	if err != nil {
		tmpDir = "/tmp/app-" + app.ID + "/" + time.Now().Format("2006-01-02-15-04-05")
		os.MkdirAll(tmpDir, os.ModePerm)
	}
	defer os.RemoveAll(tmpDir)

	repo := app.Repo()
	if repo == nil {
		return nil, errors.New("repo not found")
	}

	var stdout, stderr bytes.Buffer
	host.SetStdout(&stdout)
	host.SetStderr(&stderr)

	if err = host.Exec("bash", "-c", fmt.Sprintf(`
		cd %[1]s
		git rev-parse --short refs/heads/main
	`, repo.Path())); err != nil {
		return nil, errors.Wrap(err, "failed to get git hash")
	}

	img, err := Images.Insert(&Image{
		AppID:   app.ID,
		Status:  "building",
		GitHash: strings.TrimSpace(stdout.String()),
	})

	if err != nil {
		return nil, errors.Wrap(err, "failed to create image")
	}

	if err = host.Exec("bash", "-c", fmt.Sprintf(`
			mkdir -p %[1]s
			git clone -b main %[2]s %[1]s
			cd %[1]s
			docker build -t %[3]s:5000/%[4]s:%[5]s .
			docker push %[3]s:5000/%[4]s:%[5]s
		`, tmpDir, repo.Path(), os.Getenv("HQ_ADDR"), app.ID, img.GitHash)); err != nil {
		img.Status = "failed"
		img.Error = stderr.String()
		Images.Update(img)
		err = errors.Wrap(err, stdout.String())
		return nil, errors.Wrap(err, "failed to build image")
	}

	img.Status = "ready"
	return img, Images.Update(img)
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

// MigrateToProject converts this app and its repo into a unified Project.
// It creates a new project, migrates all related data, and cleans up the old records.
func (a *App) MigrateToProject() (*Project, error) {
	repo := a.Repo()
	if repo == nil {
		return nil, errors.New("repo not found for this app")
	}

	// Use repo.ID as project ID to keep git path consistent
	projectID := repo.ID

	// Check if project already exists
	if _, err := Projects.Get(projectID); err == nil {
		return nil, errors.New("a project with this ID already exists")
	}

	// Create the project (don't init git - repo already exists)
	project := &Project{
		Model:             application.Model{ID: projectID},
		OwnerID:           repo.OwnerID,
		Name:              a.Name,
		Description:       a.Description,
		Status:            a.Status,
		Error:             a.Error,
		OAuthClientSecret: a.OAuthClientSecret,
		DatabaseEnabled:   a.DatabaseEnabled,
	}

	// Map old status to project status
	if project.Status == "" {
		project.Status = "draft"
	}

	if _, err := Projects.Insert(project); err != nil {
		return nil, errors.Wrap(err, "failed to create project")
	}

	// Migrate Images: update ProjectID for all images with this AppID
	DB.Query("UPDATE images SET ProjectID = ? WHERE AppID = ?", projectID, a.ID).Exec()

	// Migrate Stars: copy repo stars to project
	DB.Query("UPDATE stars SET ProjectID = ? WHERE RepoID = ?", projectID, repo.ID).Exec()

	// Migrate OAuth Authorizations: update ProjectID for all with this AppID
	DB.Query("UPDATE oauth_authorizations SET ProjectID = ? WHERE AppID = ?", projectID, a.ID).Exec()

	// Migrate Comments: update SubjectID from app.ID to project.ID
	if a.ID != projectID {
		DB.Query("UPDATE comments SET SubjectID = ? WHERE SubjectID = ?", projectID, a.ID).Exec()
	}
	// Also migrate repo comments
	if repo.ID != projectID {
		DB.Query("UPDATE comments SET SubjectID = ? WHERE SubjectID = ?", projectID, repo.ID).Exec()
	}

	// Migrate Activities: update SubjectType and SubjectID
	DB.Query("UPDATE activities SET SubjectType = 'project', SubjectID = ? WHERE SubjectType = 'app' AND SubjectID = ?", projectID, a.ID).Exec()
	DB.Query("UPDATE activities SET SubjectType = 'project', SubjectID = ? WHERE SubjectType = 'repo' AND SubjectID = ?", projectID, repo.ID).Exec()

	// Create migration activity
	Activities.Insert(&Activity{
		UserID:      repo.OwnerID,
		Action:      "migrated",
		SubjectType: "project",
		SubjectID:   projectID,
		Content:     fmt.Sprintf("Migrated from app '%s' and repo '%s'", a.Name, repo.Name),
	})

	// Delete the old app (keep repo for now until we confirm it works)
	Apps.Delete(a)

	// Archive the repo instead of deleting (safer)
	repo.Archived = true
	Repos.Update(repo)

	return project, nil
}
