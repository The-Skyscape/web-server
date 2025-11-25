package controllers

import (
	"net/http"
	"strconv"

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

	page := c.defaultPage
	if pageStr := c.URL.Query().Get("page"); pageStr != "" {
		if val, err := strconv.Atoi(pageStr); err == nil && val > 0 {
			page = val
		}
	}

	limit := c.defaultLimit
	if limitStr := c.URL.Query().Get("limit"); limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			limit = val
		}
	}

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
	if page := c.URL.Query().Get("page"); page != "" {
		if val, err := strconv.Atoi(page); err == nil && val > 0 {
			return val
		}
	}
	return c.defaultPage
}

func (c *UsersController) Limit() int {
	limit := c.defaultLimit
	if limitStr := c.URL.Query().Get("limit"); limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			limit = val
		}
	}
	return min(limit, 100)
}

func (c *UsersController) NextPage() int {
	return c.Page() + 1
}
