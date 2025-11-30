package controllers

import (
	"errors"
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/models"
)

func Reactions() (string, application.Handler) {
	return "reactions", &ReactionsController{}
}

type ReactionsController struct {
	application.Controller
}

func (c *ReactionsController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := app.Use("auth").(*AuthController)

	http.Handle("POST /post/{post}/react", c.ProtectFunc(c.react, auth.Required))
	http.Handle("DELETE /post/{post}/react", c.ProtectFunc(c.unreact, auth.Required))
}

func (c ReactionsController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *ReactionsController) react(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	activityID := r.PathValue("post")
	emoji := r.FormValue("emoji")

	if activityID == "" || emoji == "" {
		c.Render(w, r, "error-message.html", errors.New("missing required fields"))
		return
	}

	// Validate emoji is a supported reaction
	if !models.IsValidReaction(emoji) {
		c.Render(w, r, "error-message.html", errors.New("invalid reaction type"))
		return
	}

	// Check if activity exists
	_, err = models.Activities.Get(activityID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("post not found"))
		return
	}

	// Check if user already has a reaction on this post
	existing, _ := models.Reactions.First("WHERE ActivityID = ? AND UserID = ?", activityID, user.ID)

	if existing != nil {
		// Update existing reaction
		existing.Emoji = emoji
		if err = models.Reactions.Update(existing); err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}
	} else {
		// Create new reaction
		_, err = models.Reactions.Insert(&models.Reaction{
			UserID:     user.ID,
			ActivityID: activityID,
			Emoji:      emoji,
		})
		if err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}
	}

	c.Refresh(w, r)
}

func (c *ReactionsController) unreact(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	activityID := r.PathValue("post")

	// Find and delete the user's reaction
	reaction, err := models.Reactions.First("WHERE ActivityID = ? AND UserID = ?", activityID, user.ID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("reaction not found"))
		return
	}

	if err = models.Reactions.Delete(reaction); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}
