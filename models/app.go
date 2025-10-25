package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

type App struct {
	application.Model
	RepoID      string
	Name        string
	Description string
}

func (*App) Table() string { return "apps" }

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
