package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/emailing"
	"www.theskyscape.com/models"
)

func Messages() (string, *MessagesController) {
	return "messages", &MessagesController{
		defaultPage:  1,
		defaultLimit: 20,
	}
}

type MessagesController struct {
	application.Controller
	defaultPage  int
	defaultLimit int
}

func (c *MessagesController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := c.Use("auth").(*AuthController)

	http.Handle("GET /messages", app.Serve("messages.html", auth.Required))
	http.Handle("GET /messages/{user}", c.ProtectFunc(c.viewConversation, auth.Required))
	http.Handle("POST /messages/{user}", c.ProtectFunc(c.sendMessage, auth.Required))
}

func (c MessagesController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *MessagesController) CurrentUser() *models.Profile {
	profile := c.Use("profile").(*ProfileController)
	return profile.CurrentProfile()
}

// Conversations returns all profiles the current user has messaged with
func (c *MessagesController) Conversations() []*models.Profile {
	currentProfile := c.CurrentUser()
	if currentProfile == nil {
		return nil
	}

	return currentProfile.MyConversations()
}

// ConversationWith returns paginated messages between current user and specified user
func (c *MessagesController) ConversationWith() []*models.Message {
	currentProfile := c.CurrentUser()
	otherProfile := c.OtherUser()

	if currentProfile == nil || otherProfile == nil {
		return nil
	}

	page := c.Page()
	limit := c.Limit()
	offset := (page - 1) * limit

	messages, _ := models.Messages.Search(`
		WHERE (SenderID = ? AND RecipientID = ?)
		   OR (SenderID = ? AND RecipientID = ?)
		ORDER BY CreatedAt DESC
		LIMIT ? OFFSET ?
	`, currentProfile.UserID, otherProfile.UserID, otherProfile.UserID, currentProfile.UserID, limit, offset)
	return messages
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

func (c *MessagesController) viewConversation(w http.ResponseWriter, r *http.Request) {
	currentProfile := c.CurrentUser()
	otherProfile := c.OtherUser()

	// Mark messages as read in a goroutine (side effect)
	if currentProfile != nil && otherProfile != nil {
		go currentProfile.MarkMessagesReadFrom(otherProfile)
	}

	// Render the conversation page
	c.Render(w, r, "conversation.html", nil)
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
			emailing.WithData("Title", "New Message"),
			emailing.WithData("recipient", otherUser),
			emailing.WithData("sender", senderProfile),
			emailing.WithData("year", time.Now().Year()),
		)
	}

	c.Refresh(w, r)
}

func (c *MessagesController) Page() int {
	page := c.defaultPage
	if pageStr := c.URL.Query().Get("page"); pageStr != "" {
		if val, err := strconv.Atoi(pageStr); err == nil && val > 0 {
			page = val
		}
	}
	return page
}

func (c *MessagesController) Limit() int {
	limit := c.defaultLimit
	if limitStr := c.URL.Query().Get("limit"); limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			limit = val
		}
	}
	return limit
}

func (c *MessagesController) NextPage() int {
	return c.Page() + 1
}
