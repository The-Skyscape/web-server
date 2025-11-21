package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

type Follow struct {
	application.Model
	FollowerID string // User who is following
	FolloweeID string // User being followed
}

func (*Follow) Table() string {
	return "follows"
}

func (f *Follow) Follower() *Profile {
	profile, _ := Profiles.Get(f.FollowerID)
	return profile
}

func (f *Follow) Followee() *Profile {
	profile, _ := Profiles.Get(f.FolloweeID)
	return profile
}
