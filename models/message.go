package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

type Message struct {
	application.Model
	SenderID    string
	RecipientID string
	Content     string
	Read        bool // Whether the message has been read
}

func (*Message) Table() string {
	return "messages"
}

// Sender returns the user who sent the message
func (m *Message) Sender() *Profile {
	profile, _ := Profiles.Get(m.SenderID)
	return profile
}

// Recipient returns the user who receives the message
func (m *Message) Recipient() *Profile {
	profile, _ := Profiles.Get(m.RecipientID)
	return profile
}

// IsUnread returns true if the message hasn't been read yet
func (m *Message) IsUnread() bool {
	return !m.Read
}

// MarkAsRead marks the message as read
func (m *Message) MarkAsRead() error {
	m.Read = true
	return Messages.Update(m)
}
