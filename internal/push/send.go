package push

import (
	"encoding/json"
	"io"
	"log"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"www.theskyscape.com/models"
)

const (
	// TTL is the time-to-live for push notifications (24 hours)
	TTL = 60 * 60 * 24
	// Subscriber is the contact email for VAPID
	Subscriber = "hello@theskyscape.com"
	// DefaultTag is the notification tag for grouping
	DefaultTag = "skyscape-message"
)

// Subscription represents a push subscription for sending
type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

// SendResult contains the result of a send attempt
type SendResult struct {
	StatusCode   int
	ShouldRemove bool   // true if subscription is invalid (410/404)
	Error        error
	ErrorBody    string // response body on error
}

// SendNotification sends a push notification to a user (rate limited per source)
func SendNotification(userID, sourceID, title, body, url string) error {
	log.Printf("[Push] Notification requested for user %s from source %s: %s", userID, sourceID, title)

	if !KeysConfigured() {
		log.Println("[Push] VAPID keys not configured, skipping push")
		return nil
	}

	// Get all subscriptions for this user
	subscriptions, err := models.PushSubscriptions.Search("WHERE UserID = ?", userID)
	if err != nil {
		log.Printf("[Push] Error fetching subscriptions: %v", err)
		return err
	}
	if len(subscriptions) == 0 {
		log.Printf("[Push] No subscriptions found for user %s", userID)
		return nil
	}
	log.Printf("[Push] Found %d subscription(s) for user %s", len(subscriptions), userID)

	// Check rate limiting
	lastLog, _ := models.PushNotificationLogs.First("WHERE UserID = ? AND SourceID = ?", userID, sourceID)

	var lastSent *time.Time
	if lastLog != nil {
		lastSent = &lastLog.LastSentAt
	}

	if !ShouldSend(lastSent) {
		log.Printf("[Push] Rate limited for source %s - last notification sent at %s",
			sourceID, lastLog.LastSentAt.Format(time.RFC3339))
		return nil
	}

	// Count messages since last notification for aggregation
	sinceTime := GetSinceTime(lastSent)
	messageCount := models.Messages.Count("WHERE RecipientID = ? AND SenderID = ? AND CreatedAt > ?",
		userID, sourceID, sinceTime)

	// Aggregate message if multiple
	notificationTitle, notificationBody, notificationURL := AggregateMessage(
		messageCount, title, body, url)

	log.Printf("[Push] Sending notification to user %s (%d messages since %s)",
		userID, messageCount, sinceTime.Format(time.RFC3339))

	// Build payload
	payload := BuildPayload(notificationTitle, notificationBody, notificationURL)

	// Send to all subscriptions
	for _, sub := range subscriptions {
		result := Send(&Subscription{
			Endpoint: sub.Endpoint,
			P256dh:   sub.P256dh,
			Auth:     sub.Auth,
		}, payload)

		endpoint := TruncateEndpoint(sub.Endpoint)

		if result.Error != nil {
			log.Printf("[Push] Failed to send to %s: %v", endpoint, result.Error)
			continue
		}

		if result.ShouldRemove {
			log.Printf("[Push] Removing invalid subscription: %d", result.StatusCode)
			models.PushSubscriptions.Delete(sub)
		} else if result.StatusCode >= 200 && result.StatusCode < 300 {
			log.Printf("[Push] Successfully sent to %s (status: %d)", endpoint, result.StatusCode)
		} else {
			log.Printf("[Push] Unexpected status %d for %s: %s",
				result.StatusCode, endpoint, result.ErrorBody)
		}
	}

	// Update the notification log
	now := time.Now()
	if lastLog != nil {
		lastLog.LastSentAt = now
		models.PushNotificationLogs.Update(lastLog)
	} else {
		models.PushNotificationLogs.Insert(&models.PushNotificationLog{
			UserID:     userID,
			SourceID:   sourceID,
			LastSentAt: now,
		})
	}

	return nil
}

// BuildPayload creates the JSON payload for a push notification
func BuildPayload(title, body, url string) []byte {
	payload := map[string]interface{}{
		"title": title,
		"body":  body,
		"url":   url,
		"tag":   DefaultTag,
	}
	bytes, _ := json.Marshal(payload)
	return bytes
}

// Send sends a push notification to a subscription
func Send(sub *Subscription, payload []byte) *SendResult {
	if !KeysConfigured() {
		return &SendResult{Error: nil} // silently skip if not configured
	}

	s := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}

	resp, err := webpush.SendNotification(payload, s, &webpush.Options{
		Subscriber:      Subscriber,
		VAPIDPublicKey:  GetPublicKey(),
		VAPIDPrivateKey: GetPrivateKey(),
		TTL:             TTL,
	})

	if err != nil {
		return &SendResult{Error: err}
	}
	defer resp.Body.Close()

	result := &SendResult{StatusCode: resp.StatusCode}

	// Check if subscription is invalid (410 Gone or 404)
	if resp.StatusCode == 410 || resp.StatusCode == 404 {
		result.ShouldRemove = true
	} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read error response body
		bodyBytes, _ := io.ReadAll(resp.Body)
		result.ErrorBody = string(bodyBytes)
	}

	return result
}

// TruncateEndpoint returns a truncated endpoint for logging
func TruncateEndpoint(endpoint string) string {
	if len(endpoint) > 50 {
		return endpoint[:50]
	}
	return endpoint
}
