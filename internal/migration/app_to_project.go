package migration

import (
	"fmt"
	"os"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/pkg/errors"
	"www.theskyscape.com/models"
)

const gitReposPath = "/mnt/git-repos"

// ErrIDConflict is returned when the project ID conflicts with existing resources
var ErrIDConflict = errors.New("id_conflict")

// CheckMigrationConflict checks if the given project ID would conflict with existing resources.
// Returns nil if no conflict, or an error describing the conflict.
func CheckMigrationConflict(projectID string) error {
	// Check if project already exists
	if _, err := models.Projects.Get(projectID); err == nil {
		return errors.Wrap(ErrIDConflict, "a project with this ID already exists")
	}

	// Check if git path would conflict
	newGitPath := fmt.Sprintf("%s/%s", gitReposPath, projectID)
	if _, err := os.Stat(newGitPath); err == nil {
		return errors.Wrap(ErrIDConflict, "git path already exists for this ID")
	}

	return nil
}

// MigrateAppToProject converts an app and its repo into a unified Project.
// It creates a new project, migrates all related data, and cleans up the old records.
// If customID is empty, the app.ID is used as the project ID.
func MigrateAppToProject(app *models.App, customID string) (*models.Project, error) {
	repo := app.Repo()
	if repo == nil {
		return nil, errors.New("repo not found for this app")
	}

	// Use custom ID if provided, otherwise use app.ID
	projectID := customID
	if projectID == "" {
		projectID = app.ID
	}

	// Check for conflicts
	if err := CheckMigrationConflict(projectID); err != nil {
		return nil, err
	}

	// Move git repo from repo path to project path
	oldGitPath := fmt.Sprintf("%s/%s", gitReposPath, repo.ID)
	newGitPath := fmt.Sprintf("%s/%s", gitReposPath, projectID)
	if err := os.Rename(oldGitPath, newGitPath); err != nil {
		return nil, errors.Wrap(err, "failed to move git repo")
	}

	// Create the project (don't init git - repo already exists)
	project := &models.Project{
		Model:             application.Model{ID: projectID},
		OwnerID:           repo.OwnerID,
		Name:              app.Name,
		Description:       app.Description,
		Status:            app.Status,
		Error:             app.Error,
		OAuthClientSecret: app.OAuthClientSecret,
		DatabaseEnabled:   app.DatabaseEnabled,
	}

	// Map old status to project status
	if project.Status == "" {
		project.Status = "draft"
	}

	if _, err := models.Projects.Insert(project); err != nil {
		return nil, errors.Wrap(err, "failed to create project")
	}

	// Migrate Images: update ProjectID for all images with this AppID
	// Since projectID == app.ID, this just sets ProjectID = AppID
	models.DB.Query("UPDATE images SET ProjectID = ? WHERE AppID = ?", projectID, app.ID).Exec()

	// Migrate Stars: copy repo stars to project
	models.DB.Query("UPDATE stars SET ProjectID = ? WHERE RepoID = ?", projectID, repo.ID).Exec()

	// Migrate OAuth Authorizations: update ProjectID for all with this AppID
	models.DB.Query("UPDATE oauth_authorizations SET ProjectID = ? WHERE AppID = ?", projectID, app.ID).Exec()

	// Migrate Comments: update SubjectID from repo.ID to project.ID
	// App comments already have correct SubjectID since projectID == app.ID
	models.DB.Query("UPDATE comments SET SubjectID = ? WHERE SubjectID = ?", projectID, repo.ID).Exec()

	// Migrate Activities: update SubjectType and SubjectID
	// App activities already have correct SubjectID, just update type
	models.DB.Query("UPDATE activities SET SubjectType = 'project' WHERE SubjectType = 'app' AND SubjectID = ?", projectID).Exec()
	models.DB.Query("UPDATE activities SET SubjectType = 'project', SubjectID = ? WHERE SubjectType = 'repo' AND SubjectID = ?", projectID, repo.ID).Exec()

	// Create migration activity
	models.Activities.Insert(&models.Activity{
		UserID:      repo.OwnerID,
		Action:      "migrated",
		SubjectType: "project",
		SubjectID:   projectID,
		Content:     fmt.Sprintf("Migrated from app '%s' and repo '%s'", app.Name, repo.Name),
	})

	// Delete the old app (keep repo for now until we confirm it works)
	models.Apps.Delete(app)

	// Archive the repo instead of deleting (safer)
	repo.Archived = true
	models.Repos.Update(repo)

	return project, nil
}
