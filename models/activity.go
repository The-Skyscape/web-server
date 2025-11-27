package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

type Activity struct {
	application.Model
	UserID      string
	Action      string
	SubjectType string
	SubjectID   string
	Content     string
	FileID      string
}

func (*Activity) Table() string { return "activities" }

func (a *Activity) User() *authentication.User {
	user, err := Auth.Users.Get(a.UserID)
	if err != nil {
		return nil
	}

	return user
}

func (a *Activity) Profile() *Profile {
	profile, err := Profiles.Get(a.SubjectID)
	if err != nil {
		return nil
	}

	return profile
}

func (a *Activity) Repo() *Repo {
	// Only return repo if SubjectType is "repo" or empty (for backwards compatibility with legacy data)
	if a.SubjectType != "" && a.SubjectType != "repo" {
		return nil
	}
	repo, err := Repos.Get(a.SubjectID)
	if err != nil {
		return nil
	}

	return repo
}

func (a *Activity) App() *App {
	// Only return app if SubjectType is "app"
	if a.SubjectType != "app" {
		return nil
	}
	app, err := Apps.Get(a.SubjectID)
	if err != nil {
		return nil
	}

	return app
}

func (a *Activity) File() *File {
	if a.FileID == "" {
		return nil
	}
	file, err := Files.Get(a.FileID)
	if err != nil {
		return nil
	}
	return file
}
