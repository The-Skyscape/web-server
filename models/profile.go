package models

import (
	"cmp"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

type Profile struct {
	application.Model
	UserID      string
	Description string
}

func (*Profile) Table() string { return "profiles" }

func (p *Profile) Apps() []any {
	return nil
}

func (p *Profile) Repos() []*Repo {
	repos, _ := Repos.Search("WHERE OwnerID = ? ORDER BY CreatedAt", p.UserID)
	return repos
}

func (p *Profile) User() *authentication.User {
	user, _ := Auth.Users.Get(p.UserID)
	return user
}

func (p *Profile) Name() string {
	return cmp.Or(p.User(), &authentication.User{}).Name
}

func (p *Profile) Handle() string {
	return cmp.Or(p.User(), &authentication.User{}).Handle
}

func (p *Profile) Avatar() string {
	return cmp.Or(p.User(), &authentication.User{}).Avatar
}
