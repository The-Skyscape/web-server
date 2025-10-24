package controllers

import (
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/models"
)

func Feed() (string, *FeedController) {
	return "feed", &FeedController{}
}

type FeedController struct {
	application.Controller
}

func (c *FeedController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := c.Use("auth").(*AuthController)

	http.Handle("/", app.Serve("tbd.html", auth.Required))
	http.Handle("/{$}", app.ProtectFunc(c.serveFeed, auth.Optional))
	http.Handle("/explore", app.Serve("explore.html", auth.Optional))
}

func (c FeedController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *FeedController) serveFeed(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	if user, _, _ := auth.Authenticate(r); user == nil {
		c.Render(w, r, "signup.html", nil)
		return
	}

	profile := c.Use("profile").(*ProfileController)
	profile.Request = r
	if profile.CurrentProfile() == nil {
		c.Render(w, r, "setup.html", nil)
		return
	}

	c.Render(w, r, "feed.html", nil)
}

func (c *FeedController) RecentActivities() []*models.Activity {
	activities, _ := models.Activities.Search(`ORDER BY CreatedAt DESC`)
	return activities
}
