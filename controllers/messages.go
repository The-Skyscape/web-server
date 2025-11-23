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

func (c *MessagesController) CurrentUser() *models.Profile {
	profile := c.Use("profile").(*ProfileController)
	return profile.CurrentProfile()
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
	currentProfile := c.CurrentUser()
	if currentProfile == nil {
		return nil
	}

	// Get all conversation partners using profile method
	partners := currentProfile.MyConversations()

	// Build conversation list with metadata
	conversations := make([]*Conversation, 0, len(partners))
	for _, partner := range partners {
		lastMsg := currentProfile.LastMessage(partner)
		if lastMsg == nil {
			continue
		}

		conversations = append(conversations, &Conversation{
			User:          partner,
			LastMessage:   lastMsg,
			UnreadCount:   currentProfile.UnreadMessagesFrom(partner),
			LastMessageAt: currentProfile.LastMessageAt(partner),
		})
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
	currentProfile := c.CurrentUser()
	otherProfile := c.OtherUser()

	if currentProfile == nil || otherProfile == nil {
		return nil
	}

	return currentProfile.ConversationWith(otherProfile)
}

// OtherUser returns the profile of the user being messaged
func (c *MessagesController) OtherUser() *models.Profile {
	otherUserHandle := c.PathValue("user")
	otherUser, _ := models.Auth.Users.First("WHERE Handle = ?", otherUserHandle)
	if otherUser == nil {
		return nil
	}
	profile, _ := models.Profiles.Get(otherUser.ID)
	return profile
}

// UnreadCount returns the total number of unread messages for the current user
func (c *MessagesController) UnreadCount() int {
	currentProfile := c.CurrentUser()
	if currentProfile == nil {
		return 0
	}

	return models.Messages.Count("WHERE RecipientID = ? AND Read = ?", currentProfile.UserID, false)
}

func (c *MessagesController) sendMessage(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	otherUserHandle := r.PathValue("user")
	otherUser, _ := models.Auth.Users.First("WHERE Handle = ?", otherUserHandle)
	if otherUser == nil {
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
		RecipientID: otherUser.ID,
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
	`, otherUser.ID, oneHourAgo)

	// If this is the only message in the last hour (count = 1, the one we just sent), send email
	if recentMessages == 1 {
		senderProfile, _ := models.Profiles.Get(user.ID)
		go models.Emails.Send(otherUser.Email,
			"New Message from "+user.Handle,
			emailing.WithTemplate("new-message.html"),
			emailing.WithData("recipient", otherUser),
			emailing.WithData("sender", senderProfile),
			emailing.WithData("year", time.Now().Year()),
		)
	}

	c.Refresh(w, r)
}

func (c *MessagesController) markAsRead(w http.ResponseWriter, r *http.Request) {
	currentProfile := c.CurrentUser()
	otherProfile := c.OtherUser()

	if currentProfile == nil || otherProfile == nil {
		c.Render(w, r, "error-message.html", errors.New("user not found"))
		return
	}

	// Mark all unread messages from other user as read using profile method
	if err := currentProfile.MarkMessagesReadFrom(otherProfile); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	w.WriteHeader(http.StatusOK)
}
