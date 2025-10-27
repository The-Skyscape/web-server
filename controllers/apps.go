package controllers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/models"
)

func Apps() (string, *AppsController) {
	return "apps", &AppsController{}
}

type AppsController struct {
	application.Controller
}

func (c *AppsController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := c.Use("auth").(*AuthController)

	http.Handle("/app/{app}", c.Serve("app.html", auth.Required))
	http.Handle("POST /apps", c.ProtectFunc(c.create, auth.Required))
	http.Handle("POST /app/{app}/launch", c.ProtectFunc(c.launch, auth.Required))
}

func (c AppsController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *AppsController) CurrentApp() *models.App {
	app, err := models.Apps.Get(c.Request.PathValue("app"))
	if err != nil {
		return nil
	}

	return app
}

func (c *AppsController) create(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	repo, err := models.Repos.Get(r.FormValue("repo"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("repo not found"))
		return
	} else if repo.OwnerID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("you are not the owner"))
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")

	if name == "" || description == "" {
		c.Render(w, r, "error-message.html", errors.New("missing name or desc"))
		return
	}

	id := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	if _, err = models.Apps.Insert(&models.App{
		Model:       application.Model{ID: id},
		Name:        name,
		Description: description,
		RepoID:      repo.ID,
	}); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Redirect(w, r, "/apps/"+id)
}

func (c *AppsController) launch(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	app, err := models.Apps.Get(r.PathValue("app"))
	log.Println("App:", app)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("app not found"))
		return
	}

	repo := app.Repo()
	log.Println("Repo:", repo)
	if repo == nil || repo.OwnerID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("app not found"))
		return
	}

	go func() {
		app.Status = "launching"
		app.Error = ""
		models.Apps.Update(app)

		if _, err := app.Build(); err != nil {
			app.Error = err.Error()
			models.Apps.Update(app)
			return
		}

		// Deploy?
		// if err = img.Deploy(host); err != nil {
		// 	app.Error = err.Error()
		// 	models.Apps.Update(app)
		// 	return
		// }
	}()

	c.Refresh(w, r)
}
