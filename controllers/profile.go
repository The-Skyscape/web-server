package controllers

import (
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"www.theskyscape.com/models"
)

func Profile() (string, *ProfileController) {
	return "profile", &ProfileController{}
}

type ProfileController struct {
	application.Controller
}

func (c *ProfileController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := c.Use("auth").(*AuthController)

	http.Handle("/profile", app.Serve("profile.html", auth.Optional))
	http.Handle("/user/{id}", app.Serve("profile.html", auth.Optional))
}

func (c ProfileController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *ProfileController) CurrentProfile() *authentication.User {
	if c.PathValue("id") == "" {
		auth := c.Use("auth").(*AuthController)
		return auth.CurrentUser()
	}

	user, err := models.Auth.Users.Get(c.PathValue("id"))
	if err != nil {
		return nil
	}

	return user
}
