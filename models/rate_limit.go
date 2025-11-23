package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

type RateLimit struct {
	application.Model
	Identifier string    // IP address or user identifier
	Action     string    // "signin", "signup", "password-reset"
	Attempts   int       // Number of attempts
	ResetAt    time.Time // When the limit resets
}

func (*RateLimit) Table() string {
	return "rate_limits"
}

// Check checks if the rate limit has been exceeded for the given identifier and action
func Check(identifier, action string, maxAttempts int, window time.Duration) (bool, int, error) {
	// Clean up expired rate limits - IMPORTANT: actually delete them to prevent orphan data
	expired, _ := RateLimits.Search("WHERE ResetAt < ?", time.Now())
	for _, limit := range expired {
		RateLimits.Delete(limit)
	}

	// Get existing rate limit record (don't create if not exists)
	limit, err := RateLimits.First("WHERE Identifier = ? AND Action = ?", identifier, action)
	if err != nil {
		// No existing limit - allow the action
		return true, maxAttempts, nil
	}

	// Check if limit has expired and should be deleted
	if time.Now().After(limit.ResetAt) {
		RateLimits.Delete(limit)
		return true, maxAttempts, nil
	}

	// Check if limit exceeded
	remaining := maxAttempts - limit.Attempts
	if remaining <= 0 {
		return false, 0, nil
	}

	return true, remaining, nil
}

// Record records an attempt for the given identifier and action
func Record(identifier, action string, window time.Duration) error {
	limit, err := RateLimits.First("WHERE Identifier = ? AND Action = ?", identifier, action)
	if err != nil {
		// Create new record
		_, err = RateLimits.Insert(&RateLimit{
			Identifier: identifier,
			Action:     action,
			Attempts:   1,
			ResetAt:    time.Now().Add(window),
		})
		return err
	}

	// Update existing record
	limit.Attempts++
	return RateLimits.Update(limit)
}

// Reset resets the rate limit for the given identifier and action
func Reset(identifier, action string) error {
	limit, err := RateLimits.First("WHERE Identifier = ? AND Action = ?", identifier, action)
	if err != nil {
		return nil // No limit to reset
	}

	return RateLimits.Delete(limit)
}
