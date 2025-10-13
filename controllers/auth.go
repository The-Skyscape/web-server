package controllers

import (
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"www.theskyscape.com/models"
)

func Auth() (string, *AuthController) {
	return "auth", &AuthController{
		models.Auth.Controller(
			authentication.WithCookie("theskyscape"),
		),
	}
}

type AuthController struct {
	*authentication.Controller
}

func (c *AuthController) Setup(app *application.App) {
	c.Controller.Setup(app)

	http.Handle("/signin", app.ProtectFunc(c.signin, nil))
	http.Handle("/signup", app.ProtectFunc(c.signup, nil))
}

func (c AuthController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *AuthController) signin(w http.ResponseWriter, r *http.Request) {
	if user, _, _ := c.Authenticate(r); user != nil {
		c.Redirect(w, r, "/")
		return
	}

	c.Render(w, r, "signin.html", nil)
}

func (c *AuthController) signup(w http.ResponseWriter, r *http.Request) {
	if user, _, _ := c.Authenticate(r); user != nil {
		c.Redirect(w, r, "/")
		return
	}

	c.Render(w, r, "signup.html", nil)
}
