package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/models"
)

func Feed() (string, *FeedController) {
	return "feed", &FeedController{
		defaultPage:  1,
		defaultLimit: 10,
	}
}

type FeedController struct {
	application.Controller
	defaultPage  int
	defaultLimit int
}

func (c *FeedController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := c.Use("auth").(*AuthController)

	http.Handle("/", app.Serve("tbd.html", auth.Required))
	http.Handle("/{$}", app.ProtectFunc(c.serveFeed, auth.Optional))
	http.Handle("/explore", app.Serve("explore.html", auth.Optional))
	http.Handle("/manifesto", app.Serve("manifesto.html", auth.Optional))
	http.Handle("POST /feed/post", c.ProtectFunc(c.createPost, auth.Required))
	http.Handle("DELETE /feed/{post}", c.ProtectFunc(c.deletePost, auth.Required))
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

func (c *FeedController) Page() int {
	page := c.defaultPage
	if pageStr := c.URL.Query().Get("page"); pageStr != "" {
		if val, err := strconv.Atoi(pageStr); err == nil && val > 0 {
			page = val
		}
	}
	return page
}

func (c *FeedController) Limit() int {
	limit := c.defaultLimit
	if limitStr := c.URL.Query().Get("limit"); limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			limit = val
		}
	}
	return limit
}

func (c *FeedController) NextPage() int {
	return c.Page() + 1
}

func (c *FeedController) RecentActivities() []*models.Activity {
	page := c.Page()
	limit := c.Limit()
	offset := (page - 1) * limit

	activities, _ := models.Activities.Search(`
		ORDER BY CreatedAt DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	return activities
}

func (c *FeedController) createPost(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	content := r.FormValue("content")
	if content == "" {
		c.Render(w, r, "error-message.html", errors.New("Post content cannot be empty"))
		return
	}

	_, err = models.Activities.Insert(&models.Activity{
		UserID:      user.ID,
		Action:      "posted",
		SubjectType: "",
		SubjectID:   "",
		Content:     content,
	})
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}

func (c *FeedController) deletePost(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	post, err := models.Activities.Get(r.PathValue("post"))
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	if !user.IsAdmin && post.UserID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("Not allowed"))
		return
	}

	if err = models.Activities.Delete(post); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}
