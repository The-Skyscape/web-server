package controllers

import (
	"errors"
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/models"
)

func Stars() (string, application.Handler) {
	return "stars", &StarsController{}
}

type StarsController struct {
	application.Controller
}

func (c *StarsController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := app.Use("auth").(*AuthController)

	http.Handle("POST /repo/{repo}/star", c.ProtectFunc(c.star, auth.Required))
	http.Handle("DELETE /repo/{repo}/star", c.ProtectFunc(c.unstar, auth.Required))
}

func (c StarsController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *StarsController) star(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	repoID := r.PathValue("repo")

	// Check if already starred
	existing, _ := models.Stars.First("WHERE UserID = ? AND RepoID = ?",
		user.ID, repoID)
	if existing != nil {
		c.Render(w, r, "error-message.html", errors.New("already starred"))
		return
	}

	// Get the repo to ensure it exists
	repo, err := models.Repos.Get(repoID)
	if err != nil || repo == nil {
		c.Render(w, r, "error-message.html", errors.New("repository not found"))
		return
	}

	// Create star
	_, err = models.Stars.Insert(&models.Star{
		UserID: user.ID,
		RepoID: repoID,
	})
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Create activity for feed
	models.Activities.Insert(&models.Activity{
		UserID:      user.ID,
		Action:      "starred",
		SubjectType: "repo",
		SubjectID:   repoID,
	})

	c.Refresh(w, r)
}

func (c *StarsController) unstar(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	repoID := r.PathValue("repo")

	star, err := models.Stars.First("WHERE UserID = ? AND RepoID = ?",
		user.ID, repoID)
	if err != nil || star == nil {
		c.Render(w, r, "error-message.html", errors.New("not starred"))
		return
	}

	if err = models.Stars.Delete(star); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}
