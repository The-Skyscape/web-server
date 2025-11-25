package controllers

import (
	"cmp"
	"errors"
	"net/http"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/models"
)

func Comments() (string, application.Handler) {
	return "comments", &CommentsController{}
}

type CommentsController struct {
	application.Controller
}

func (c *CommentsController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := app.Use("auth").(*AuthController)

	http.Handle("POST /comment", c.ProtectFunc(c.create, auth.Required))
	http.Handle("PUT /comment/{comment}", c.ProtectFunc(c.update, auth.Required))
	http.Handle("DELETE /comment/{comment}", c.ProtectFunc(c.delete, auth.Required))
}

func (c CommentsController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *CommentsController) create(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	subjectID := r.FormValue("subject_id")
	subjectType := r.FormValue("subject_type")
	content := r.FormValue("content")

	if subjectID == "" || content == "" {
		c.Render(w, r, "error-message.html", errors.New("missing required fields"))
		return
	}

	if len(content) > 10000 {
		c.Render(w, r, "error-message.html", errors.New("comment too long, max 10000 characters"))
		return
	}

	_, err = models.Comments.Insert(&models.Comment{
		UserID:    user.ID,
		SubjectID: subjectID,
		Content:   content,
	})
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Create activity for the comment
	var activitySubjectID string
	activitySubjectType := "repo"

	if subjectType == "file" {
		// Extract repo ID from "file:{repo_id}:{path}" format
		parts := strings.SplitN(subjectID, ":", 3)
		if len(parts) >= 2 {
			activitySubjectID = parts[1]
		}
	} else {
		activitySubjectID = subjectID
	}

	if activitySubjectID != "" {
		models.Activities.Insert(&models.Activity{
			UserID:      user.ID,
			Action:      "commented",
			SubjectType: activitySubjectType,
			SubjectID:   activitySubjectID,
			Content:     content,
		})
	}

	c.Refresh(w, r)
}

func (c *CommentsController) update(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	comment, err := models.Comments.Get(r.PathValue("comment"))
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	if comment.UserID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("not authorized"))
		return
	}

	comment.Content = cmp.Or(r.Header.Get("HX-Prompt"), comment.Content)
	if err = models.Comments.Update(comment); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}

func (c *CommentsController) delete(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	comment, err := models.Comments.Get(r.PathValue("comment"))
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	if comment.UserID != user.ID && !user.IsAdmin {
		c.Render(w, r, "error-message.html", errors.New("not authorized"))
		return
	}

	if err = models.Comments.Delete(comment); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}
