package controllers

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/emailing"
	"www.theskyscape.com/models"
)

func Auth() (string, *AuthController) {
	return "auth", &AuthController{
		models.Auth.Controller(
			authentication.WithCookie("theskyscape"),
			authentication.WithSignupHandler(func(c *authentication.Controller, user *authentication.User) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					// In the background;
					go func() {
						// Welcome the new user to The Skyscape community
						models.Emails.Send(user.Email,
							"Welcome to The Skyscape",
							emailing.WithTemplate("welcome.html"),
							emailing.WithData("user", user),
							emailing.WithData("year", time.Now().Year()),
						)

						// Notify other users that The Skyscape has grown
						users, _ := models.Auth.Users.Search("WHERE ID != ?", user.ID)
						for _, u := range users {
							models.Emails.Send(u.Email,
								"The Skyscape Has Grown",
								emailing.WithTemplate("new-user.html"),
								emailing.WithData("user", user),
								emailing.WithData("year", time.Now().Year()),
							)
						}
					}()

					// While we redirect the user to their profile
					c.Redirect(w, r, "/profile")
				}
			}),
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
	"web.skysca.pe", // legacy
	"www.skysca.pe",
	"theskyscape.com",
	"www.theskyscape.com",
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
