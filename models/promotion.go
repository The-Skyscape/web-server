package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// DefaultPromotionDuration is how long promotions are visible (7 days)
const DefaultPromotionDuration = 7 * 24 * time.Hour

type Promotion struct {
	application.Model
	UserID      string
	SubjectType string // "repo" or "app"
	SubjectID   string
	Content     string
	ExpiresAt   time.Time
}

func (*Promotion) Table() string {
	return "promotions"
}

func (p *Promotion) User() *authentication.User {
	user, _ := Auth.Users.Get(p.UserID)
	return user
}

func (p *Promotion) Profile() *Profile {
	profile, _ := Profiles.First("WHERE UserID = ?", p.UserID)
	return profile
}

func (p *Promotion) Repo() *Repo {
	if p.SubjectType != "repo" {
		return nil
	}
	repo, _ := Repos.Get(p.SubjectID)
	return repo
}

func (p *Promotion) App() *App {
	if p.SubjectType != "app" {
		return nil
	}
	app, _ := Apps.Get(p.SubjectID)
	return app
}

func (p *Promotion) IsExpired() bool {
	return time.Now().After(p.ExpiresAt)
}

func (p *Promotion) DaysRemaining() int {
	remaining := time.Until(p.ExpiresAt)
	if remaining <= 0 {
		return 0
	}
	return int(remaining.Hours() / 24)
}

// ActivePromotions returns all non-expired promotions ordered by creation date
func ActivePromotions() []*Promotion {
	promotions, _ := Promotions.Search(`
		WHERE ExpiresAt > ?
		ORDER BY CreatedAt DESC
	`, time.Now())
	return promotions
}
