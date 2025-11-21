package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

type Star struct {
	application.Model
	UserID string
	RepoID string
}

func (*Star) Table() string {
	return "stars"
}

func (s *Star) User() *Profile {
	profile, _ := Profiles.Get(s.UserID)
	return profile
}

func (s *Star) Repo() *Repo {
	repo, _ := Repos.Get(s.RepoID)
	return repo
}
