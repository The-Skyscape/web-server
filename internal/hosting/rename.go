package hosting

import (
	"fmt"
	"log"
	"os"

	"github.com/pkg/errors"
	"www.theskyscape.com/models"
)

const gitReposPath = "/mnt/git-repos"

// RenameApp changes an app's ID and updates all related tables.
// Apps don't have their own git repos (they use repos), so no file move is needed.
func RenameApp(oldID, newID, name, description string) error {
	// Update app ID
	if err := models.DB.Query(
		"UPDATE apps SET ID = ?, Name = ?, Description = ? WHERE ID = ?",
		newID, name, description, oldID,
	).Exec(); err != nil {
		return errors.New("an app with this ID already exists")
	}

	// Update related tables with AppID column
	updateAppRelatedTables(oldID, newID)

	// Update subject tables
	updateSubjectTables("app", oldID, newID)

	return nil
}

// RenameProject changes a project's ID, moves the git repo, and updates all related tables.
func RenameProject(oldID, newID, name, description string) error {
	oldGitPath := fmt.Sprintf("%s/%s", gitReposPath, oldID)
	newGitPath := fmt.Sprintf("%s/%s", gitReposPath, newID)

	// Move git repo to new path
	if err := os.Rename(oldGitPath, newGitPath); err != nil {
		log.Printf("[ProjectRename] Failed to move git repo from %s to %s: %v", oldGitPath, newGitPath, err)
		return errors.Wrap(err, "failed to move git repo")
	}

	// Update project ID
	if err := models.DB.Query(
		"UPDATE projects SET ID = ?, Name = ?, Description = ? WHERE ID = ?",
		newID, name, description, oldID,
	).Exec(); err != nil {
		// Rollback git move
		os.Rename(newGitPath, oldGitPath)
		return errors.New("a project with this ID already exists")
	}

	// Update related tables with ProjectID column
	updateProjectRelatedTables(oldID, newID)

	// Update subject tables
	updateSubjectTables("project", oldID, newID)

	return nil
}

// updateAppRelatedTables updates all tables that reference an app by AppID
func updateAppRelatedTables(oldID, newID string) {
	tables := []struct {
		table  string
		column string
	}{
		{"images", "AppID"},
		{"app_metrics", "AppID"},
		{"oauth_authorizations", "AppID"},
		{"oauth_authorization_codes", "ClientID"},
	}

	for _, t := range tables {
		if err := models.DB.Query(
			fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s = ?", t.table, t.column, t.column),
			newID, oldID,
		).Exec(); err != nil {
			log.Printf("[AppRename] Failed to update %s.%s from %s to %s: %v", t.table, t.column, oldID, newID, err)
		}
	}
}

// updateProjectRelatedTables updates all tables that reference a project by ProjectID
func updateProjectRelatedTables(oldID, newID string) {
	tables := []struct {
		table  string
		column string
	}{
		{"images", "ProjectID"},
		{"app_metrics", "ProjectID"},
		{"oauth_authorizations", "ProjectID"},
		{"stars", "ProjectID"},
	}

	for _, t := range tables {
		if err := models.DB.Query(
			fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s = ?", t.table, t.column, t.column),
			newID, oldID,
		).Exec(); err != nil {
			log.Printf("[ProjectRename] Failed to update %s.%s from %s to %s: %v", t.table, t.column, oldID, newID, err)
		}
	}
}

// updateSubjectTables updates all tables that reference an entity as a subject
func updateSubjectTables(subjectType, oldID, newID string) {
	// Activities and promotions filter by SubjectType
	subjectTypeTables := []string{"activities", "promotions"}
	for _, table := range subjectTypeTables {
		if err := models.DB.Query(
			fmt.Sprintf("UPDATE %s SET SubjectID = ? WHERE SubjectType = ? AND SubjectID = ?", table),
			newID, subjectType, oldID,
		).Exec(); err != nil {
			log.Printf("[%sRename] Failed to update %s.SubjectID from %s to %s: %v", subjectType, table, oldID, newID, err)
		}
	}

	// Comments don't have SubjectType - update all matching SubjectIDs
	if err := models.DB.Query(
		"UPDATE comments SET SubjectID = ? WHERE SubjectID = ?",
		newID, oldID,
	).Exec(); err != nil {
		log.Printf("[%sRename] Failed to update comments.SubjectID from %s to %s: %v", subjectType, oldID, newID, err)
	}
}
