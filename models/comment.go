package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

type Comment struct {
	application.Model
	UserID    string
	SubjectID string
	Content   string
}

func (*Comment) Table() string {
	return "comments"
}

func (c *Comment) User() *authentication.User {
	user, _ := Auth.Users.Get(c.UserID)
	return user
}

// UserProfile returns the profile of the user who wrote this comment
func (c *Comment) UserProfile() *Profile {
	profile, _ := Profiles.First("WHERE UserID = ?", c.UserID)
	return profile
}
