package controllers

import (
	"errors"
	"net/http"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/emailing"
	"www.theskyscape.com/models"
)

func Follows() (string, application.Handler) {
	return "follows", &FollowsController{}
}

type FollowsController struct {
	application.Controller
}

func (c *FollowsController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := app.Use("auth").(*AuthController)

	http.Handle("POST /user/{user}/follow", c.ProtectFunc(c.follow, auth.Required))
	http.Handle("DELETE /user/{user}/follow", c.ProtectFunc(c.unfollow, auth.Required))
}

func (c FollowsController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *FollowsController) follow(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	followeeID := r.PathValue("user")

	// Validate not following self
	if user.ID == followeeID {
		c.Render(w, r, "error-message.html", errors.New("cannot follow yourself"))
		return
	}

	// Check if already following
	existing, _ := models.Follows.First("WHERE FollowerID = ? AND FolloweeID = ?",
		user.ID, followeeID)
	if existing != nil {
		c.Render(w, r, "error-message.html", errors.New("already following"))
		return
	}

	// Get the followee to ensure they exist
	followee, err := models.Auth.Users.Get(followeeID)
	if err != nil || followee == nil {
		c.Render(w, r, "error-message.html", errors.New("user not found"))
		return
	}

	// Create follow
	_, err = models.Follows.Insert(&models.Follow{
		FollowerID: user.ID,
		FolloweeID: followeeID,
	})
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Create activity
	models.Activities.Insert(&models.Activity{
		UserID:      user.ID,
		Action:      "followed",
		SubjectType: "profile",
		SubjectID:   followeeID,
	})

	// Send email notification in background
	go func() {
		models.Emails.Send(followee.Email,
			"New Follower on The Skyscape",
			emailing.WithTemplate("new-follower.html"),
			emailing.WithData("user", followee),
			emailing.WithData("follower", user),
			emailing.WithData("year", time.Now().Year()),
		)
	}()

	c.Refresh(w, r)
}

func (c *FollowsController) unfollow(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	followeeID := r.PathValue("user")

	follow, err := models.Follows.First("WHERE FollowerID = ? AND FolloweeID = ?",
		user.ID, followeeID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("not following"))
		return
	}

	if err = models.Follows.Delete(follow); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}
