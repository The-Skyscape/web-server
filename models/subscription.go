package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// ProductType identifies the type of subscription
const (
	ProductVerified     = "verified"
	ProductAppResources = "app_resources"
)

// SubscriptionStatus represents the state of a subscription
const (
	StatusActive   = "active"
	StatusPastDue  = "past_due"
	StatusCanceled = "canceled"
	StatusTrialing = "trialing"
)

type Subscription struct {
	application.Model
	UserID               string
	StripeCustomerID     string
	StripeSubscriptionID string
	ProductType          string // "verified", "app_resources"
	SubjectID            string // App ID for resources, empty for verified
	Status               string // "active", "past_due", "canceled", "trialing"
	CurrentPeriodEnd     time.Time
	CanceledAt           *time.Time
}

func (*Subscription) Table() string { return "subscriptions" }

// User returns the subscription owner
func (s *Subscription) User() *authentication.User {
	user, _ := Auth.Users.Get(s.UserID)
	return user
}

// App returns the app if this is an app_resources subscription
func (s *Subscription) App() *App {
	if s.ProductType != ProductAppResources || s.SubjectID == "" {
		return nil
	}
	app, _ := Apps.Get(s.SubjectID)
	return app
}

// IsActive returns true if the subscription is active or trialing
func (s *Subscription) IsActive() bool {
	return s.Status == StatusActive || s.Status == StatusTrialing
}

// GetUserVerifiedSubscription returns the active verified subscription for a user
func GetUserVerifiedSubscription(userID string) *Subscription {
	sub, err := Subscriptions.First(`
		WHERE UserID = ? AND ProductType = ? AND Status IN (?, ?)
	`, userID, ProductVerified, StatusActive, StatusTrialing)
	if err != nil {
		return nil
	}
	return sub
}

// GetAppResourceSubscription returns the active resource subscription for an app
func GetAppResourceSubscription(appID string) *Subscription {
	sub, err := Subscriptions.First(`
		WHERE SubjectID = ? AND ProductType = ? AND Status IN (?, ?)
	`, appID, ProductAppResources, StatusActive, StatusTrialing)
	if err != nil {
		return nil
	}
	return sub
}

// UserSubscriptions returns all subscriptions for a user
func UserSubscriptions(userID string, limit int) []*Subscription {
	subs, _ := Subscriptions.Search(`
		WHERE UserID = ?
		ORDER BY CreatedAt DESC
		LIMIT ?
	`, userID, limit)
	return subs
}
