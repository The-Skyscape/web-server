package controllers

import (
	"encoding/json"
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
	http.Handle("GET /messages/{id}", c.ProtectFunc(c.viewConversation, auth.Required))
	http.Handle("GET /messages/{id}/list", app.Serve("message-list", auth.Required))
	http.Handle("GET /messages/{id}/poll", c.ProtectFunc(c.pollMessages, auth.Required))
	http.Handle("POST /messages/{id}", c.ProtectFunc(c.sendMessage, auth.Required))
	http.Handle("GET /api/messages/unread", c.ProtectFunc(c.apiUnreadCount, auth.Required))
}

func (c MessagesController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *MessagesController) CurrentUser() *models.Profile {
	auth := c.Use("auth").(*AuthController)
	user := auth.CurrentUser()
	if user == nil {
		return nil
	}

	profile, _ := models.Profiles.Get(user.ID)
	return profile
}

func (c *MessagesController) CurrentProfile() *models.Profile {
	profile := c.Use("profile").(*ProfileController)
	return profile.CurrentProfile()
}

func (c *MessagesController) Count() int {
	profile := c.CurrentProfile()
	if profile == nil {
		return 0
	}

	return profile.MessageCount(c.CurrentUser())
}

func (c *MessagesController) Messages() []*models.Message {
	profile := c.CurrentProfile()
	if profile == nil {
		return nil
	}

	return profile.Messages(c.CurrentUser(), c.defaultPage, c.defaultLimit)
}

func (c *MessagesController) Conversations() []*models.Profile {
	user := c.CurrentUser()
	if user == nil {
		return nil
	}

	return user.MyConversations()
}

func (c *MessagesController) UnreadCount() int {
	user := c.CurrentUser()
	if user == nil {
		return 0
	}

	return models.Messages.Count(`
		WHERE RecipientID = ?
			AND Read = false
	`, user.ID)
}

// apiUnreadCount returns JSON with unread message count for polling
func (c *MessagesController) apiUnreadCount(w http.ResponseWriter, r *http.Request) {
	c.Request = r

	count := c.UnreadCount()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":       count,
		"lastChecked": time.Now().UTC().Format(time.RFC3339),
	})
}

func (c MessagesController) viewConversation(w http.ResponseWriter, r *http.Request) {
	c.Request = r

	user := c.CurrentUser()
	profile := c.CurrentProfile()
	if user != nil && profile != nil {
		user.MarkMessagesReadFrom(profile)
	}

	c.Render(w, r, "conversation.html", nil)
}

// pollMessages returns new messages since the given timestamp
func (c MessagesController) pollMessages(w http.ResponseWriter, r *http.Request) {
	c.Request = r

	user := c.CurrentUser()
	profile := c.CurrentProfile()
	if user == nil || profile == nil {
		return
	}

	// Parse the 'after' timestamp (Unix seconds)
	afterStr := r.URL.Query().Get("after")
	var after time.Time
	if afterStr != "" {
		if unix, err := strconv.ParseInt(afterStr, 10, 64); err == nil {
			after = time.Unix(unix, 0)
		}
	}

	// Get new messages from the other person (incoming only)
	newMessages, _ := models.Messages.Search(`
		WHERE SenderID = ? AND RecipientID = ? AND CreatedAt > ?
		ORDER BY CreatedAt ASC
	`, profile.ID, user.ID, after)

	// Mark them as read
	if len(newMessages) > 0 {
		user.MarkMessagesReadFrom(profile)
	}

	// Render the new messages
	c.Render(w, r, "message-poll.html", newMessages)
}

func (c MessagesController) sendMessage(w http.ResponseWriter, r *http.Request) {
	c.Request = r

	user := c.CurrentUser()
	if user == nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	profile, err := models.Profiles.Get(r.FormValue("id"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("user not found"))
		return
	}

	content := r.FormValue("content")
	if content == "" {
		c.Render(w, r, "error-message.html", errors.New("message cannot be empty"))
		return
	}
	if len(content) > 10000 {
		c.Render(w, r, "error-message.html", errors.New("message too long"))
		return
	}

	// Create the message
	_, err = models.Messages.Insert(&models.Message{
		SenderID:    user.ID,
		RecipientID: profile.ID,
		Content:     content,
	})
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Send push notification to recipient
	go models.SendPushNotification(
		profile.ID,
		user.ID, // source = sender
		"New message from @"+user.Handle(),
		truncateMessage(content, 100),
		"/messages/"+user.ID,
	)

	// Check if we should send email notification
	// Only send if this is the first message received in the last hour
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	recentMessages := models.Messages.Count(`
		WHERE RecipientID = ? AND CreatedAt > ?
	`, profile.ID, oneHourAgo)

	// If this is the only message in the last hour (count = 1, the one we just sent), send email
	if recentMessages == 1 {
		userProfile, _ := models.Profiles.Get(user.ID)
		go models.Emails.Send(profile.User().Email,
			"New Message from "+user.Handle(),
			emailing.WithTemplate("new-message.html"),
			emailing.WithData("Title", "New Message"),
			emailing.WithData("recipient", profile),
			emailing.WithData("sender", userProfile),
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
	return min(limit, 100)
}

func (c *MessagesController) NextPage() int {
	return c.Page() + 1
}

// truncateMessage truncates a message to maxLen characters with ellipsis
func truncateMessage(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
