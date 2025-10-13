package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

func (*Repo) Table() string { return "repos" }

type Repo struct {
	application.Model
	Name    string
	OwnerID string
}

func (r *Repo) Owner() *authentication.User {
	u, err := Auth.Users.Get(r.OwnerID)
	if err != nil {
		return nil
	}

	return u
}
