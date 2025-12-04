package controllers

import (
	"cmp"
	"errors"
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
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

	http.Handle("GET /profile", app.Serve("profile.html", auth.Required))
	http.Handle("GET /user/{id}", app.Serve("profile.html", auth.Optional))
	http.Handle("GET /user/{id}/repos", app.Serve("user-repos.html", auth.Optional))
	http.Handle("GET /user/{id}/apps", app.Serve("user-apps.html", auth.Optional))
	http.Handle("GET /user/{id}/followers", app.Serve("user-followers.html", auth.Optional))
	http.Handle("GET /user/{id}/following", app.Serve("user-following.html", auth.Optional))
	http.Handle("POST /setup", app.ProtectFunc(c.setup, auth.Optional))
}

func (c ProfileController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *ProfileController) CurrentProfile() *models.Profile {
	if c.PathValue("id") == "" {
		auth := c.Use("auth").(*AuthController)
		user := auth.CurrentUser()
		if user == nil {
			return nil
		}
		p, err := models.Profiles.Get(user.ID)
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

func (c *ProfileController) GetProfile(userID string) *models.Profile {
	p, err := models.Profiles.Get(userID)
	if err != nil {
		return nil
	}
	return p
}

func (p *ProfileController) RecentProfiles() []*models.Profile {
	query := p.URL.Query().Get("query")
	profiles, _ := models.Profiles.Search(`
	  INNER JOIN users on users.ID = profiles.UserID
		WHERE
			users.Name           LIKE $1        OR
			users.Handle         LIKE LOWER($1) OR
			profiles.Description LIKE $1
		ORDER BY (SELECT COUNT(*) FROM follows WHERE FolloweeID = profiles.ID) DESC
		LIMIT 4
	`, "%"+query+"%")
	return profiles
}

func (c *ProfileController) setup(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	desc := r.FormValue("description")
	if p, err := models.Profiles.Get(user.ID); err != nil {
		if _, err = models.CreateProfile(user.ID, desc); err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}
	} else {
		user := p.User()
		user.Avatar = cmp.Or(r.FormValue("avatar"), user.Avatar)
		user.Name = cmp.Or(r.FormValue("name"), user.Name)
		if err = models.Auth.Users.Update(user); err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}

		p.Description = cmp.Or(desc, p.Description)
		if err = models.Profiles.Update(p); err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}
	}

	c.Refresh(w, r)
}
