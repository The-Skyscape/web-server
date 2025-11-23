package controllers

import (
	"errors"
	"net/http"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/emailing"
	"www.theskyscape.com/models"
)

func Messages() (string, *MessagesController) {
	return "messages", &MessagesController{}
}

type MessagesController struct {
	application.Controller
}

func (c *MessagesController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := c.Use("auth").(*AuthController)

	http.Handle("GET /messages", app.Serve("messages.html", auth.Required))
	http.Handle("GET /messages/{user}", app.Serve("conversation.html", auth.Required))
	http.Handle("POST /messages/{user}", c.ProtectFunc(c.sendMessage, auth.Required))
	http.Handle("POST /messages/{user}/mark-read", c.ProtectFunc(c.markAsRead, auth.Required))
}

func (c MessagesController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

// Conversations returns a list of users the current user has conversations with
// along with the most recent message and unread count
type Conversation struct {
	User          *models.Profile
	LastMessage   *models.Message
	UnreadCount   int
	LastMessageAt time.Time
}

func (c *MessagesController) Conversations() []*Conversation {
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(c.Request)
	if user == nil {
		return nil
	}

	// Get all unique users the current user has messaged with
	messages, _ := models.Messages.Search(`
		WHERE SenderID = ? OR RecipientID = ?
		ORDER BY CreatedAt DESC
	`, user.ID, user.ID)

	// Group by conversation partner
	conversationMap := make(map[string]*Conversation)
	for _, msg := range messages {
		var partnerID string
		if msg.SenderID == user.ID {
			partnerID = msg.RecipientID
		} else {
			partnerID = msg.SenderID
		}

		if _, exists := conversationMap[partnerID]; !exists {
			profile, _ := models.Profiles.Get(partnerID)
			if profile == nil {
				continue
			}

			// Count unread messages from this partner
			unreadCount := models.Messages.Count(`
				WHERE SenderID = ? AND RecipientID = ? AND Read = ?
			`, partnerID, user.ID, false)

			conversationMap[partnerID] = &Conversation{
				User:          profile,
				LastMessage:   msg,
				UnreadCount:   unreadCount,
				LastMessageAt: msg.CreatedAt,
			}
		}
	}

	// Convert map to slice and sort by last message time
	conversations := make([]*Conversation, 0, len(conversationMap))
	for _, conv := range conversationMap {
		conversations = append(conversations, conv)
	}

	// Sort by last message time (most recent first)
	for i := 0; i < len(conversations); i++ {
		for j := i + 1; j < len(conversations); j++ {
			if conversations[j].LastMessageAt.After(conversations[i].LastMessageAt) {
				conversations[i], conversations[j] = conversations[j], conversations[i]
			}
		}
	}

	return conversations
}

// ConversationWith returns all messages between current user and specified user
func (c *MessagesController) ConversationWith() []*models.Message {
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(c.Request)
	if user == nil {
		return nil
	}

	otherUserHandle := c.PathValue("user")
	otherProfile, _ := models.Profiles.First("WHERE Handle = ?", otherUserHandle)
	if otherProfile == nil {
		return nil
	}

	messages, _ := models.Messages.Search(`
		WHERE (SenderID = ? AND RecipientID = ?)
		   OR (SenderID = ? AND RecipientID = ?)
		ORDER BY CreatedAt ASC
	`, user.ID, otherProfile.UserID, otherProfile.UserID, user.ID)

	return messages
}

// OtherUser returns the profile of the user being messaged
func (c *MessagesController) OtherUser() *models.Profile {
	otherUserHandle := c.PathValue("user")
	otherProfile, _ := models.Profiles.First("WHERE Handle = ?", otherUserHandle)
	return otherProfile
}

// UnreadCount returns the total number of unread messages for the current user
func (c *MessagesController) UnreadCount() int {
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(c.Request)
	if user == nil {
		return 0
	}

	return models.Messages.Count("WHERE RecipientID = ? AND Read = ?", user.ID, false)
}

func (c *MessagesController) sendMessage(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	otherUserHandle := r.PathValue("user")
	otherProfile, _ := models.Profiles.First("WHERE Handle = ?", otherUserHandle)
	if otherProfile == nil {
		c.Render(w, r, "error-message.html", errors.New("user not found"))
		return
	}

	content := r.FormValue("content")
	if content == "" {
		c.Render(w, r, "error-message.html", errors.New("message cannot be empty"))
		return
	}

	// Create the message
	_, err = models.Messages.Insert(&models.Message{
		SenderID:    user.ID,
		RecipientID: otherProfile.UserID,
		Content:     content,
	})
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Check if we should send email notification
	// Only send if this is the first message received in the last hour
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	recentMessages := models.Messages.Count(`
		WHERE RecipientID = ? AND CreatedAt > ?
	`, otherProfile.UserID, oneHourAgo)

	// If this is the only message in the last hour (count = 1, the one we just sent), send email
	if recentMessages == 1 {
		recipient, _ := models.Auth.Users.Get(otherProfile.UserID)
		if recipient != nil {
			senderProfile, _ := models.Profiles.Get(user.ID)
			go models.Emails.Send(recipient.Email,
				"New Message from "+user.Handle,
				emailing.WithTemplate("new-message.html"),
				emailing.WithData("recipient", recipient),
				emailing.WithData("sender", senderProfile),
				emailing.WithData("year", time.Now().Year()),
			)
		}
	}

	c.Refresh(w, r)
}

func (c *MessagesController) markAsRead(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	otherUserHandle := r.PathValue("user")
	otherProfile, _ := models.Profiles.First("WHERE Handle = ?", otherUserHandle)
	if otherProfile == nil {
		c.Render(w, r, "error-message.html", errors.New("user not found"))
		return
	}

	// Mark all unread messages from this user as read
	messages, _ := models.Messages.Search(`
		WHERE SenderID = ? AND RecipientID = ? AND Read = ?
	`, otherProfile.UserID, user.ID, false)

	for _, msg := range messages {
		msg.Read = true
		models.Messages.Update(msg)
	}

	w.WriteHeader(http.StatusOK)
}
