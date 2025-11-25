package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// Call represents an audio call between two users
type Call struct {
	application.Model
	CallerID  string    // User who initiated the call
	CalleeID  string    // User receiving the call
	Status    string    // pending, ringing, active, ended
	StartedAt time.Time // When call was answered
	EndedAt   time.Time // When call ended
	Duration  int       // Call duration in seconds
	EndReason string    // completed, cancelled, rejected, missed, failed
}

func (*Call) Table() string {
	return "calls"
}

// Caller returns the profile of the user who initiated the call
func (c *Call) Caller() *Profile {
	profile, _ := Profiles.Get(c.CallerID)
	return profile
}

// Callee returns the profile of the user receiving the call
func (c *Call) Callee() *Profile {
	profile, _ := Profiles.Get(c.CalleeID)
	return profile
}

// IsActive returns true if the call is currently in progress
func (c *Call) IsActive() bool {
	return c.Status == "active"
}

// IsPending returns true if waiting for the callee to answer
func (c *Call) IsPending() bool {
	return c.Status == "pending" || c.Status == "ringing"
}

// IsEnded returns true if the call has ended
func (c *Call) IsEnded() bool {
	return c.Status == "ended"
}

// End marks the call as ended with the given reason
func (c *Call) End(reason string) error {
	c.Status = "ended"
	c.EndedAt = time.Now()
	c.EndReason = reason
	if !c.StartedAt.IsZero() {
		c.Duration = int(c.EndedAt.Sub(c.StartedAt).Seconds())
	}
	return Calls.Update(c)
}

// Accept marks the call as active (answered)
func (c *Call) Accept() error {
	c.Status = "active"
	c.StartedAt = time.Now()
	return Calls.Update(c)
}
