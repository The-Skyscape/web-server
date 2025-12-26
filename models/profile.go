package models

import (
	"cmp"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
)

type Profile struct {
	application.Model
	UserID           string
	Description      string
	Verified         bool   // User has active Verified subscription
	StripeCustomerID string // Stripe customer ID for billing
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

// Followers returns users following this profile (max 1000)
func (p *Profile) Followers() []*Follow {
	follows, _ := Follows.Search(`
		WHERE FolloweeID = ?
		ORDER BY CreatedAt DESC
		LIMIT 1000
	`, p.UserID)
	return follows
}

// Following returns users this profile follows (max 1000)
func (p *Profile) Following() []*Follow {
	follows, _ := Follows.Search(`
		WHERE FollowerID = ?
		ORDER BY CreatedAt DESC
		LIMIT 1000
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

// AppsCount returns the count of active apps owned by this profile
func (p *Profile) AppsCount() int {
	return Apps.Count(`
		JOIN repos ON repos.ID = apps.RepoID
		WHERE repos.OwnerID = ? AND apps.Status != 'shutdown'
	`, p.UserID)
}

// ReposCount returns the count of non-archived repos owned by this profile
func (p *Profile) ReposCount() int {
	return Repos.Count("WHERE OwnerID = ? AND Archived = false", p.UserID)
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

	// Create "joined" activity for the new user
	Activities.Insert(&Activity{
		UserID:      userID,
		Action:      "joined",
		SubjectType: "profile",
		SubjectID:   userID,
	})

	return p, err
}

func (p *Profile) MessageCount(with *Profile) int {
	if with == nil {
		return 0
	}

	return Messages.Count(`
		WHERE (SenderID = ? AND RecipientID = ?)
		   OR (SenderID = ? AND RecipientID = ?)
	`, p.ID, with.ID, with.ID, p.ID)
}

// LastMessage returns the most recent message between this profile and another
func (p *Profile) LastMessage(with *Profile) *Message {
	if with == nil {
		return nil
	}

	message, err := Messages.First(`
		WHERE (SenderID = ? AND RecipientID = ?)
		   OR (SenderID = ? AND RecipientID = ?)
		ORDER BY CreatedAt DESC
	`, p.ID, with.ID, with.ID, p.ID)

	if err != nil {
		return nil
	}

	return message
}

func (p *Profile) Messages(with *Profile, page, limit int) []*Message {
	if with == nil {
		return nil
	}

	messages, _ := Messages.Search(`
		WHERE (SenderID = ? AND RecipientID = ?)
		   OR (SenderID = ? AND RecipientID = ?)
		ORDER BY CreatedAt DESC
		LIMIT ? OFFSET ?
	`, p.ID, with.ID, with.ID, p.ID, limit, (page-1)*limit)
	return messages
}

// UnreadMessagesFrom returns count of unread messages FROM another profile TO this profile
func (p *Profile) UnreadMessagesFrom(from *Profile) int {
	return Messages.Count(`
		WHERE SenderID = ? AND RecipientID = ? AND Read = false
	`, from.ID, p.ID)
}

// LastMessageAt returns the timestamp of the last message between profiles
func (p *Profile) LastMessageAt(with *Profile) time.Time {
	message := p.LastMessage(with)
	if message == nil {
		return time.Time{}
	}
	return message.CreatedAt
}

// MyConversations returns profiles this user has exchanged messages with (max 50)
func (p *Profile) MyConversations() []*Profile {
	profiles, _ := Profiles.Search(`
		JOIN messages ON (
			(messages.SenderID = profiles.ID AND messages.RecipientID = ?)
			OR
			(messages.RecipientID = profiles.ID AND messages.SenderID = ?)
		)
		GROUP BY profiles.ID
		ORDER BY MAX(messages.CreatedAt) DESC
		LIMIT 50
	`, p.ID, p.ID)

	return profiles
}

// MarkMessagesReadFrom marks all unread messages from another profile as read
func (p *Profile) MarkMessagesReadFrom(from *Profile) error {
	messages, _ := Messages.Search(`
		WHERE SenderID = ? AND RecipientID = ? AND Read = false
	`, from.ID, p.ID)

	for _, msg := range messages {
		msg.Read = true
		if err := Messages.Update(msg); err != nil {
			return err
		}
	}
	return nil
}

// RecentActivities returns the user's recent activity feed posts
func (p *Profile) RecentActivities(limit int) []*Activity {
	activities, _ := Activities.Search(`
		WHERE UserID = ?
		ORDER BY CreatedAt DESC
		LIMIT ?
	`, p.UserID, limit)
	return activities
}

// Thoughts returns published thoughts by this user (max 100)
func (p *Profile) Thoughts() []*Thought {
	thoughts, _ := Thoughts.Search(`
		WHERE UserID = ? AND Published = true
		ORDER BY CreatedAt DESC
		LIMIT 100
	`, p.UserID)
	return thoughts
}

// AllThoughts returns thoughts by this user including drafts (max 100)
func (p *Profile) AllThoughts() []*Thought {
	thoughts, _ := Thoughts.Search(`
		WHERE UserID = ?
		ORDER BY CreatedAt DESC
		LIMIT 100
	`, p.UserID)
	return thoughts
}

// RecentThoughts returns the most recent published thoughts
func (p *Profile) RecentThoughts(limit int) []*Thought {
	thoughts, _ := Thoughts.Search(`
		WHERE UserID = ? AND Published = true
		ORDER BY CreatedAt DESC
		LIMIT ?
	`, p.UserID, limit)
	return thoughts
}

// ThoughtsCount returns the count of published thoughts
func (p *Profile) ThoughtsCount() int {
	return Thoughts.Count("WHERE UserID = ? AND Published = true", p.UserID)
}

// Projects returns all non-shutdown projects owned by this profile
func (p *Profile) Projects() []*Project {
	projects, _ := Projects.Search(`
		WHERE OwnerID = ?
		 AND Status != 'shutdown'
		ORDER BY CreatedAt DESC
	`, p.UserID)
	return projects
}

// RecentProjects returns the most recent non-shutdown projects
func (p *Profile) RecentProjects() []*Project {
	projects, _ := Projects.Search(`
		WHERE OwnerID = ?
		 AND Status != 'shutdown'
		ORDER BY CreatedAt DESC
		LIMIT 3
	`, p.UserID)
	return projects
}

// ProjectsCount returns the count of non-shutdown projects owned by this profile
func (p *Profile) ProjectsCount() int {
	return Projects.Count("WHERE OwnerID = ? AND Status != 'shutdown'", p.UserID)
}
