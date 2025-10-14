package controllers

import (
	"cmp"
	"errors"
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/database"
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

	http.Handle("/profile", app.ProtectFunc(c.profile, auth.Optional))
	http.Handle("/user/{id}", app.Serve("profile.html", auth.Optional))
	http.Handle("POST /setup", app.ProtectFunc(c.setup, auth.Optional))
}

func (c ProfileController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *ProfileController) profile(w http.ResponseWriter, r *http.Request) {
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

	c.Render(w, r, "profile.html", nil)
}

func (c *ProfileController) CurrentProfile() *models.Profile {
	if c.PathValue("id") == "" {
		auth := c.Use("auth").(*AuthController)
		p, err := models.Profiles.Get(auth.CurrentUser().ID)
		if err != nil {
			return nil
		}

		return p
	}

	user, err := models.Auth.LookupUser(c.PathValue("id"))
	if err != nil {
		return nil
	}

	p, err := models.Profiles.Get(user.ID)
	if err != nil {
		return nil
	}

	return p
}

func (c *ProfileController) setup(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	if p, err := models.Profiles.Get(user.ID); err != nil {
		_, err = models.Profiles.Insert(&models.Profile{
			Model:       database.Model{ID: user.ID},
			UserID:      user.ID,
			Description: r.FormValue("description"),
		})

		if err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}
	} else {
		user := p.User()
		user.Name = cmp.Or(r.FormValue("name"), user.Name)
		if err = models.Auth.Users.Update(user); err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}

		p.Description = cmp.Or(r.FormValue("description"), p.Description)
		if err = models.Profiles.Update(p); err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}
	}

	c.Refresh(w, r)
}
