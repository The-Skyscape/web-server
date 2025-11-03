package controllers

import (
	"cmp"
	"errors"
	"log"
	"net/http"

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

	http.Handle("PUT /comment/{comment}", c.ProtectFunc(c.update, auth.Required))
	http.Handle("DELETE /comment/{comment}", c.ProtectFunc(c.delete, auth.Required))
}

func (c CommentsController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
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

	log.Println("Deleting comment:", comment)
	if err = models.Comments.Delete(comment); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}
