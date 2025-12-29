package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// PushSubscription stores a user's web push subscription
type PushSubscription struct {
	application.Model
	UserID   string
	Endpoint string
	P256dh   string // Public key for encryption
	Auth     string // Auth secret
}

func (p *PushSubscription) Table() string {
	return "push_subscriptions"
}

// PushNotificationLog tracks when notifications were last sent to users per source
type PushNotificationLog struct {
	application.Model
	UserID     string // Recipient
	SourceID   string // Sender/poster who triggered the notification
	LastSentAt time.Time
}

func (p *PushNotificationLog) Table() string {
	return "push_notification_logs"
}

// User returns the recipient's profile
func (p *PushNotificationLog) User() *Profile {
	profile, _ := Profiles.Get(p.UserID)
	return profile
}

// Source returns the sender's profile
func (p *PushNotificationLog) Source() *Profile {
	profile, _ := Profiles.Get(p.SourceID)
	return profile
}
