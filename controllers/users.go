package controllers

import (
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/models"
)

func Users() (string, application.Handler) {
	return "users", &UsersController{
		defaultPage:  1,
		defaultLimit: 10,
	}
}

type UsersController struct {
	application.Controller
	defaultPage  int
	defaultLimit int
}

func (c *UsersController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := app.Use("auth").(*AuthController)

	http.Handle("GET /users", app.Serve("users.html", auth.Optional))
}

func (c UsersController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *UsersController) AllProfiles() []*models.Profile {
	query := c.URL.Query().Get("query")
	page := ParsePage(c.URL.Query(), c.defaultPage)
	limit := ParseLimit(c.URL.Query(), c.defaultLimit)

	users, _ := models.Profiles.Search(`
	  INNER JOIN users on users.ID = profiles.UserID
		WHERE
			users.Name           LIKE $1        OR
			users.Handle         LIKE LOWER($1) OR
			profiles.Description LIKE $1
		ORDER BY profiles.CreatedAt
		LIMIT $2 OFFSET $3
	`, "%"+query+"%", limit, (page-1)*limit)
	return users
}

func (c *UsersController) Page() int {
	return ParsePage(c.URL.Query(), c.defaultPage)
}

func (c *UsersController) Limit() int {
	return ParseLimit(c.URL.Query(), c.defaultLimit)
}

func (c *UsersController) NextPage() int {
	return c.Page() + 1
}
