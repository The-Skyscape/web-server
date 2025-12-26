package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

type Star struct {
	application.Model
	UserID    string
	RepoID    string // legacy - for Repo stars
	ProjectID string // new - for Project stars
}

func (*Star) Table() string {
	return "stars"
}

func (s *Star) User() *Profile {
	profile, _ := Profiles.Get(s.UserID)
	return profile
}

func (s *Star) Repo() *Repo {
	if s.RepoID == "" {
		return nil
	}
	repo, _ := Repos.Get(s.RepoID)
	return repo
}

func (s *Star) Project() *Project {
	if s.ProjectID == "" {
		return nil
	}
	project, _ := Projects.Get(s.ProjectID)
	return project
}
