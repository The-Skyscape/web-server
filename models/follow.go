package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

type Follow struct {
	application.Model
	FollowerID string // User who is following
	FolloweeID string // User being followed
}

func (*Follow) Table() string {
	return "follows"
}

func (f *Follow) Follower() *authentication.User {
	user, _ := Auth.Users.Get(f.FollowerID)
	return user
}

func (f *Follow) Followee() *authentication.User {
	user, _ := Auth.Users.Get(f.FolloweeID)
	return user
}

func (f *Follow) FollowerProfile() *Profile {
	profile, _ := Profiles.First("WHERE UserID = ?", f.FollowerID)
	return profile
}

func (f *Follow) FolloweeProfile() *Profile {
	profile, _ := Profiles.First("WHERE UserID = ?", f.FolloweeID)
	return profile
}
