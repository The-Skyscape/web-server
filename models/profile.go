package models

import (
	"cmp"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
)

type Profile struct {
	application.Model
	UserID      string
	Description string
}

func (*Profile) Table() string { return "profiles" }

func (p *Profile) Apps() []*App {
	apps, _ := Apps.Search(`
		JOIN repos ON repos.ID = apps.RepoID
		WHERE repos.OwnerID = ?
		ORDER BY apps.CreatedAt DESC
	`, p.UserID)
	return apps
}

func (p *Profile) Repos() []*Repo {
	repos, _ := Repos.Search(`
		WHERE OwnerID = ?
		ORDER BY CreatedAt DESC
	`, p.UserID)
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

func CreateProfile(userID, description string) (*Profile, error) {
	p, err := Profiles.Insert(&Profile{
		Model:       database.Model{ID: userID},
		UserID:      userID,
		Description: description,
	})

	if err != nil {
		return nil, err
	}

	Activities.Insert(&Activity{
		UserID:      userID,
		Action:      "joined",
		SubjectType: "profile",
		SubjectID:   userID,
	})

	return p, err
}
