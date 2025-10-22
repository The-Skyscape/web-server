package controllers

import (
	"net/http"
	"slices"
	"strings"

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

var WebHostNames = []string{
	"skysca.pe",
	"web.skysca.pe",
	"www.skysca.pe",
}

func (c *AuthController) Optional(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	if !slices.Contains(WebHostNames, r.Host) {
		if parts := strings.Split(r.Host, "."); len(parts) == 3 {
			w.Write([]byte(parts[0]))
			return false
		}
	}

	return c.Controller.Optional(app, w, r)
}

func (c *AuthController) Required(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	if !slices.Contains(WebHostNames, r.Host) {
		if parts := strings.Split(r.Host, "."); len(parts) == 3 {
			w.Write([]byte(parts[0]))
			return false
		}
	}

	if ok := c.Controller.Required(app, w, r); !ok {
		return ok
	}

	profile := c.Use("profile").(*ProfileController)
	profile.Request = r
	if profile.CurrentProfile() == nil {
		c.Render(w, r, "setup.html", nil)
		return false
	}

	return true
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
