package controllers

import (
	"cmp"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/emailing"
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

	// Handle post comments - notify the post author
	if subjectType == "post" {
		go func() {
			activity, err := models.Activities.Get(subjectID)
			if err != nil || activity == nil {
				return
			}

			// Don't notify yourself
			if activity.UserID == user.ID {
				return
			}

			// Rate limit: 1 notification per hour per recipient
			allowed, _, _ := models.Check(activity.UserID, "comment-notification", 1, time.Hour)
			if !allowed {
				return
			}
			models.Record(activity.UserID, "comment-notification", time.Hour)

			// Get post author's profile and user
			postAuthor, _ := models.Profiles.First("WHERE UserID = ?", activity.UserID)
			if postAuthor == nil {
				return
			}
			postAuthorUser := postAuthor.User()
			if postAuthorUser == nil {
				return
			}

			// Get commenter's profile
			commenter, _ := models.Profiles.First("WHERE UserID = ?", user.ID)
			if commenter == nil {
				return
			}

			// Truncate comment for preview
			preview := content
			if len(preview) > 200 {
				preview = preview[:197] + "..."
			}

			// Send email notification
			models.Emails.Send(postAuthorUser.Email,
				"New comment on your post",
				emailing.WithTemplate("new-comment.html"),
				emailing.WithData("commenter", commenter),
				emailing.WithData("recipient", postAuthor),
				emailing.WithData("comment", preview),
				emailing.WithData("year", time.Now().Year()),
			)
		}()
	} else {
		// Create activity for non-post comments (repo/file/app comments)
		var activitySubjectID string
		activitySubjectType := subjectType

		if subjectType == "file" {
			// Extract repo ID from "file:{repo_id}:{path}" format
			parts := strings.SplitN(subjectID, ":", 3)
			if len(parts) >= 2 {
				activitySubjectID = parts[1]
				activitySubjectType = "repo"
			}
		} else if subjectType == "app" || subjectType == "repo" {
			activitySubjectID = subjectID
		} else {
			// Default to repo for backwards compatibility
			activitySubjectType = "repo"
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
