package push

import (
	"strconv"
	"time"
)

// RateLimitDuration is the minimum time between notifications per source
const RateLimitDuration = 1 * time.Hour

// ShouldSend checks if enough time has passed since the last notification
// Returns true if lastSent is nil or older than RateLimitDuration
func ShouldSend(lastSent *time.Time) bool {
	if lastSent == nil {
		return true
	}
	return time.Since(*lastSent) >= RateLimitDuration
}

// GetSinceTime returns the time to use for counting messages
// Uses lastSent if available, otherwise falls back to RateLimitDuration ago
func GetSinceTime(lastSent *time.Time) time.Time {
	if lastSent != nil {
		return *lastSent
	}
	return time.Now().Add(-RateLimitDuration)
}

// AggregateMessage determines the notification content based on message count
// For a single message, returns the original content
// For multiple messages, returns an aggregated summary
func AggregateMessage(count int, title, body, url string) (outTitle, outBody, outURL string) {
	if count <= 1 {
		return title, body, url
	}
	return "New messages", "You have " + strconv.Itoa(count) + " new messages", "/messages"
}
