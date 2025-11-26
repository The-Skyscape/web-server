package models

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
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

// PushNotificationLog tracks when notifications were last sent to users
type PushNotificationLog struct {
	application.Model
	UserID     string
	LastSentAt time.Time
}

func (p *PushNotificationLog) Table() string {
	return "push_notification_logs"
}

// SendPushNotification sends a push notification to a user (rate limited to 1 per hour)
func SendPushNotification(userID string, title string, body string, url string) error {
	log.Printf("[Push] Notification requested for user %s: %s", userID, title)

	// Get VAPID keys from environment
	vapidPublicKey := os.Getenv("VAPID_PUBLIC_KEY")
	vapidPrivateKey := os.Getenv("VAPID_PRIVATE_KEY")

	if vapidPublicKey == "" || vapidPrivateKey == "" {
		log.Println("[Push] VAPID keys not configured, skipping push")
		return nil
	}

	// Get all subscriptions for this user
	subscriptions, err := PushSubscriptions.Search("WHERE UserID = ?", userID)
	if err != nil {
		log.Printf("[Push] Error fetching subscriptions: %v", err)
		return err
	}
	if len(subscriptions) == 0 {
		return nil // No subscriptions, nothing to send
	}

	// Check rate limiting - only send one notification per hour
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	lastLog, _ := PushNotificationLogs.First("WHERE UserID = ?", userID)

	if lastLog != nil && lastLog.LastSentAt.After(oneHourAgo) {
		log.Printf("[Push] Rate limited - last notification sent at %s", lastLog.LastSentAt.Format(time.RFC3339))
		return nil
	}

	// Count unread messages since last notification
	var messageCount int
	var sinceTime time.Time
	if lastLog != nil {
		sinceTime = lastLog.LastSentAt
	} else {
		sinceTime = oneHourAgo
	}

	messageCount = Messages.Count("WHERE RecipientID = ? AND CreatedAt > ?", userID, sinceTime)

	// Build notification message
	var notificationTitle, notificationBody, notificationURL string
	if messageCount <= 1 {
		notificationTitle = title
		notificationBody = body
		notificationURL = url
	} else {
		notificationTitle = "New messages"
		notificationBody = fmt.Sprintf("You have %d new messages", messageCount)
		notificationURL = "/messages"
	}

	log.Printf("[Push] Sending notification to user %s (%d messages since %s)",
		userID, messageCount, sinceTime.Format(time.RFC3339))

	// Create notification payload
	payload := map[string]interface{}{
		"title": notificationTitle,
		"body":  notificationBody,
		"url":   notificationURL,
		"tag":   "skyscape-message",
	}
	payloadBytes, _ := json.Marshal(payload)

	// Send to all subscriptions
	for _, sub := range subscriptions {
		s := &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				P256dh: sub.P256dh,
				Auth:   sub.Auth,
			},
		}

		resp, err := webpush.SendNotification(payloadBytes, s, &webpush.Options{
			Subscriber:      "hello@theskyscape.com", // webpush-go adds mailto: automatically
			VAPIDPublicKey:  vapidPublicKey,
			VAPIDPrivateKey: vapidPrivateKey,
			TTL:             60 * 60 * 24, // 24 hours
		})

		if err != nil {
			log.Printf("[Push] Failed to send to %s: %v", sub.Endpoint[:min(50, len(sub.Endpoint))], err)
			continue
		}

		// If subscription is invalid (410 Gone or 404), remove it
		if resp.StatusCode == 410 || resp.StatusCode == 404 {
			log.Printf("[Push] Removing invalid subscription: %d", resp.StatusCode)
			PushSubscriptions.Delete(sub)
		} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("[Push] Successfully sent to %s (status: %d)", sub.Endpoint[:min(50, len(sub.Endpoint))], resp.StatusCode)
		} else {
			// Read response body for error details
			bodyBytes, _ := io.ReadAll(resp.Body)
			log.Printf("[Push] Unexpected status %d for %s: %s", resp.StatusCode, sub.Endpoint[:min(50, len(sub.Endpoint))], string(bodyBytes))
		}
		resp.Body.Close()
	}

	// Update the notification log
	now := time.Now()
	if lastLog != nil {
		lastLog.LastSentAt = now
		PushNotificationLogs.Update(lastLog)
	} else {
		PushNotificationLogs.Insert(&PushNotificationLog{
			UserID:     userID,
			LastSentAt: now,
		})
	}

	return nil
}

// GetVAPIDPublicKey returns the public key for client-side subscription
func GetVAPIDPublicKey() string {
	return os.Getenv("VAPID_PUBLIC_KEY")
}
