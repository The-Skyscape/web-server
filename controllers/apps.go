package controllers

import (
	"errors"
	"net/http"
	"time"

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

	http.Handle("GET /apps", c.Serve("apps.html", auth.Optional))
	http.Handle("/app/{app}", c.Serve("app.html", auth.Optional))
	http.Handle("POST /apps", c.ProtectFunc(c.create, auth.Required))
	http.Handle("POST /app/{app}/edit", c.ProtectFunc(c.update, auth.Required))
	http.Handle("POST /app/{app}/launch", c.ProtectFunc(c.launch, auth.Required))
	http.Handle("POST /apps/{app}/promote", c.ProtectFunc(c.promoteApp, auth.Required))
	http.Handle("DELETE /apps/{app}/promote", c.ProtectFunc(c.cancelPromotion, auth.Required))
	http.Handle("DELETE /app/{app}", c.ProtectFunc(c.shutdown, auth.Required))
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

func (c *AppsController) AuthorizedUsers() []*models.OAuthAuthorization {
	app := c.CurrentApp()
	if app == nil {
		return nil
	}

	auths, _ := models.OAuthAuthorizations.Search("WHERE AppID = ? AND Revoked = false", app.ID)
	return auths
}

func (c *AppsController) AllApps() []*models.App {
	query := c.URL.Query().Get("query")
	apps, _ := models.Apps.Search(`
		INNER JOIN repos on repos.ID = apps.RepoID
	  INNER JOIN users on users.ID = repos.OwnerID
		WHERE
			apps.Status != 'shutdown'
			AND (
				apps.Name         LIKE $1 OR
				apps.Description  LIKE $1 OR
				repos.Name        LIKE $1 OR
				repos.Description LIKE $1 OR
				users.Handle      LIKE LOWER($1)
			)
		ORDER BY repos.CreatedAt DESC
	`, "%"+query+"%")
	return apps
}

func (c *AppsController) RecentApps() []*models.App {
	query := c.URL.Query().Get("query")
	apps, _ := models.Apps.Search(`
		INNER JOIN repos on repos.ID = apps.RepoID
	  INNER JOIN users on users.ID = repos.OwnerID
		WHERE
			apps.Status != 'shutdown'
			AND (
				apps.Name         LIKE $1 OR
				apps.Description  LIKE $1 OR
				repos.Name        LIKE $1 OR
				repos.Description LIKE $1 OR
				users.Handle      LIKE LOWER($1)
			)
		ORDER BY repos.CreatedAt DESC
		LIMIT 3
	`, "%"+query+"%")
	return apps
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

	app, err := models.NewApp(repo, name, description)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Redirect(w, r, "/app/"+app.ID)
}

func (c *AppsController) update(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	app, err := models.Apps.Get(r.PathValue("app"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("app not found"))
		return
	}

	repo := app.Repo()
	if repo == nil || repo.OwnerID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("you are not the owner"))
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")

	if name == "" || description == "" {
		c.Render(w, r, "error-message.html", errors.New("missing name or description"))
		return
	}

	// Update app fields
	app.Name = name
	app.Description = description

	if err := models.Apps.Update(app); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}

func (c *AppsController) launch(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	app, err := models.Apps.Get(r.PathValue("app"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("app not found"))
		return
	}

	repo := app.Repo()
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
	}()

	time.Sleep(time.Millisecond * 250)
	c.Refresh(w, r)
}

func (c *AppsController) shutdown(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	app, err := models.Apps.Get(r.PathValue("app"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("app not found"))
		return
	}

	repo := app.Repo()
	if repo == nil || repo.OwnerID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("permission denied"))
		return
	}

	app.Status = "shutdown"
	if err = models.Apps.Update(app); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Redirect(w, r, "/profile")
}

func (c *AppsController) promoteApp(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	app, err := models.Apps.Get(r.PathValue("app"))
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	repo := app.Repo()
	if repo == nil || repo.OwnerID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("you can only promote your own apps"))
		return
	}

	// Check if app already has an active promotion
	if existing := app.ActivePromotion(); existing != nil {
		c.Render(w, r, "error-message.html", errors.New("this app already has an active promotion"))
		return
	}

	content := r.FormValue("content")
	if len(content) > MaxContentLength {
		c.Render(w, r, "error-message.html", errors.New("promotion content too long"))
		return
	}

	if _, err = models.Promotions.Insert(&models.Promotion{
		UserID:      user.ID,
		SubjectType: "app",
		SubjectID:   app.ID,
		Content:     content,
		ExpiresAt:   time.Now().Add(models.DefaultPromotionDuration),
	}); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Redirect(w, r, "/")
}

func (c *AppsController) cancelPromotion(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	app, err := models.Apps.Get(r.PathValue("app"))
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	repo := app.Repo()
	if repo == nil || repo.OwnerID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("you can only cancel your own promotions"))
		return
	}

	promo := app.ActivePromotion()
	if promo == nil {
		c.Render(w, r, "error-message.html", errors.New("no active promotion found"))
		return
	}

	if err = models.Promotions.Delete(promo); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}
