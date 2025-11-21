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
			AND apps.Status != 'shutdown'
		ORDER BY
			apps.CreatedAt DESC
	`, p.UserID)
	return apps
}

func (p *Profile) Repos() []*Repo {
	repos, _ := Repos.Search(`
		WHERE OwnerID = ?
		 AND Archived = false
		ORDER BY CreatedAt DESC
	`, p.UserID)
	return repos
}

func (p *Profile) RecentApps() []*App {
	apps, _ := Apps.Search(`
		JOIN repos ON repos.ID = apps.RepoID
		WHERE repos.OwnerID = ?
			AND apps.Status != 'shutdown'
		ORDER BY
			apps.CreatedAt DESC
		LIMIT 3
	`, p.UserID)
	return apps
}

func (p *Profile) RecentRepos() []*Repo {
	repos, _ := Repos.Search(`
		WHERE OwnerID = ?
		 AND Archived = false
		ORDER BY CreatedAt DESC
		LIMIT 3
	`, p.UserID)
	return repos
}

// Followers returns all users following this profile
func (p *Profile) Followers() []*Follow {
	follows, _ := Follows.Search(`
		WHERE FolloweeID = ?
		ORDER BY CreatedAt DESC
	`, p.UserID)
	return follows
}

// Following returns all users this profile follows
func (p *Profile) Following() []*Follow {
	follows, _ := Follows.Search(`
		WHERE FollowerID = ?
		ORDER BY CreatedAt DESC
	`, p.UserID)
	return follows
}

// RecentFollowers returns the most recent followers for avatar display
func (p *Profile) RecentFollowers(limit int) []*Follow {
	follows, _ := Follows.Search(`
		WHERE FolloweeID = ?
		ORDER BY CreatedAt DESC
		LIMIT ?
	`, p.UserID, limit)
	return follows
}

// FollowersCount returns the count of followers
func (p *Profile) FollowersCount() int {
	return Follows.Count("WHERE FolloweeID = ?", p.UserID)
}

// FollowingCount returns the count of users this profile follows
func (p *Profile) FollowingCount() int {
	return Follows.Count("WHERE FollowerID = ?", p.UserID)
}

// IsFollowedBy checks if a specific user follows this profile
func (p *Profile) IsFollowedBy(userID string) bool {
	follow, _ := Follows.First("WHERE FollowerID = ? AND FolloweeID = ?", userID, p.UserID)
	return follow != nil
}

// IsFollowing checks if this profile follows a specific user
func (p *Profile) IsFollowing(userID string) bool {
	follow, _ := Follows.First("WHERE FollowerID = ? AND FolloweeID = ?", p.UserID, userID)
	return follow != nil
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
