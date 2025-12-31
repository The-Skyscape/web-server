package controllers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/internal/hosting"
	"www.theskyscape.com/internal/migration"
	"www.theskyscape.com/internal/social"
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
	http.Handle("/app/{app}/manage", c.Serve("app-manage.html", auth.Required))
	http.Handle("/app/{app}/history", c.ProtectFunc(c.redirectToManage, auth.Optional))
	http.Handle("GET /app/{app}/versions", c.ProtectFunc(c.pollVersions, auth.Required))
	http.Handle("GET /app/{app}/comments", c.Serve("app-comments.html", auth.Optional))
	http.Handle("POST /apps", c.ProtectFunc(c.create, auth.Required))
	http.Handle("POST /app/{app}/edit", c.ProtectFunc(c.update, auth.Required))
	http.Handle("POST /app/{app}/launch", c.ProtectFunc(c.launch, auth.Required))
	http.Handle("POST /app/{app}/enable-database", c.ProtectFunc(c.enableDatabase, auth.Required))
	http.Handle("POST /apps/{app}/promote", c.ProtectFunc(c.promoteApp, auth.Required))
	http.Handle("DELETE /apps/{app}/promote", c.ProtectFunc(c.cancelPromotion, auth.Required))
	http.Handle("POST /app/{app}/share", c.ProtectFunc(c.shareApp, auth.Required))
	http.Handle("POST /app/{app}/migrate", c.ProtectFunc(c.migrateToProject, auth.Required))
	http.Handle("DELETE /app/{app}", c.ProtectFunc(c.shutdown, auth.Required))
}

func (c AppsController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *AppsController) MyApps() []*models.App {
	auth := c.Use("auth").(*AuthController)
	user := auth.CurrentUser()
	if user == nil {
		return nil
	}

	apps, _ := models.Apps.Search(`
		JOIN repos ON repos.ID = apps.RepoID
		WHERE repos.OwnerID = ? AND apps.Status != 'shutdown'
		ORDER BY apps.CreatedAt DESC
	`, user.ID)
	return apps
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

	auths, _ := models.OAuthAuthorizations.Search(`
		WHERE AppID = ?
		AND Revoked = false
	`, app.ID)
	return auths
}

func (c *AppsController) CurrentAppMetrics() *models.AppMetrics {
	app := c.CurrentApp()
	if app == nil {
		return nil
	}

	metrics, err := models.AppMetricsManager.First("WHERE AppID = ?", app.ID)
	if err != nil {
		return nil
	}

	return metrics
}

const defaultCommentLimit = 10

func (c *AppsController) CommentPage() int {
	page, _ := strconv.Atoi(c.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	return page
}

func (c *AppsController) CommentLimit() int {
	limit, _ := strconv.Atoi(c.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = defaultCommentLimit
	}
	return limit
}

func (c *AppsController) CommentNextPage() int {
	return c.CommentPage() + 1
}

func (c *AppsController) Comments() []*models.Comment {
	app := c.CurrentApp()
	if app == nil {
		return nil
	}
	limit := c.CommentLimit()
	offset := (c.CommentPage() - 1) * limit
	return app.Comments(limit, offset)
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

func (c *AppsController) ReadmeFile() *models.Blob {
	app := c.CurrentApp()
	if app == nil {
		return nil
	}

	repo := app.Repo()
	if repo == nil {
		return nil
	}

	files := []string{"README.md", "README", "readme.md", "readme"}
	for _, name := range files {
		if file, err := repo.Open("main", name); err == nil {
			return file
		}
	}

	return nil
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
		ORDER BY (SELECT COUNT(*) FROM oauth_authorizations WHERE AppID = apps.ID AND Revoked = false) DESC
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
	databaseEnabled := r.FormValue("database") == "true"

	if name == "" || description == "" {
		c.Render(w, r, "error-message.html", errors.New("missing name or desc"))
		return
	}

	// Sanitize ID
	id, err := hosting.SanitizeID(name)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Check if app already exists
	if _, err := models.Apps.Get(id); err == nil {
		c.Render(w, r, "error-message.html", errors.New("an app with this ID already exists"))
		return
	}

	// Create app record
	app, err := models.NewApp(id, repo.ID, name, description, databaseEnabled)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Create activity
	models.Activities.Insert(&models.Activity{
		UserID:      repo.OwnerID,
		Action:      "launched",
		SubjectType: "app",
		SubjectID:   app.ID,
	})

	// Trigger build in background
	go func() {
		app.Status = "launching"
		models.Apps.Update(app)

		if _, err := hosting.BuildApp(app); err != nil {
			app.Error = err.Error()
			models.Apps.Update(app)
		}
	}()

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
	isOwner := repo != nil && repo.OwnerID == user.ID

	// Allow owner or admin to edit
	if !isOwner && !user.IsAdmin {
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

	// Handle ID change (admin only)
	newID := r.FormValue("id")
	if newID != "" && newID != app.ID && user.IsAdmin {
		if err := hosting.RenameApp(app.ID, newID, name, description); err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}
		c.Redirect(w, r, "/app/"+newID+"/manage")
		return
	}

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
	isOwner := repo != nil && repo.OwnerID == user.ID
	if !isOwner && !user.IsAdmin {
		c.Render(w, r, "error-message.html", errors.New("permission denied"))
		return
	}

	go func() {
		app.Status = "launching"
		app.Error = ""
		models.Apps.Update(app)

		if _, err := hosting.BuildApp(app); err != nil {
			app.Error = err.Error()
			models.Apps.Update(app)
			return
		}
	}()

	time.Sleep(time.Millisecond * 250)
	c.Refresh(w, r)
}

func (c *AppsController) enableDatabase(w http.ResponseWriter, r *http.Request) {
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
	isOwner := repo != nil && repo.OwnerID == user.ID
	if !isOwner && !user.IsAdmin {
		c.Render(w, r, "error-message.html", errors.New("permission denied"))
		return
	}

	if app.DatabaseEnabled {
		c.Render(w, r, "error-message.html", errors.New("database already enabled"))
		return
	}

	// Enable database and trigger new build
	app.DatabaseEnabled = true
	models.Apps.Update(app)

	go func() {
		app.Status = "launching"
		app.Error = ""
		models.Apps.Update(app)

		if _, err := hosting.BuildApp(app); err != nil {
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
	isOwner := repo != nil && repo.OwnerID == user.ID
	if !isOwner && !user.IsAdmin {
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

	content := r.FormValue("content")
	if _, err := social.CreatePromotion(user.ID, social.WrapApp(app), content); err != nil {
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

	if err := social.CancelPromotion(user.ID, social.WrapApp(app)); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}

func (c *AppsController) shareApp(w http.ResponseWriter, r *http.Request) {
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

	content := r.FormValue("content")
	if len(content) > MaxContentLength {
		c.Render(w, r, "error-message.html", errors.New("content too long"))
		return
	}

	if _, err = models.Activities.Insert(&models.Activity{
		UserID:      user.ID,
		Action:      "posted",
		SubjectType: "app",
		SubjectID:   app.ID,
		Content:     content,
	}); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Redirect(w, r, "/")
}

func (c *AppsController) migrateToProject(w http.ResponseWriter, r *http.Request) {
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
	isOwner := repo != nil && repo.OwnerID == user.ID
	if !isOwner && !user.IsAdmin {
		c.Render(w, r, "error-message.html", errors.New("permission denied"))
		return
	}

	// Get custom ID from form (empty on first attempt)
	customID := r.FormValue("project_id")

	project, err := migration.MigrateAppToProject(app, customID)
	if err != nil {
		// Check if it's an ID conflict - show modal for alternative ID
		if errors.Is(err, migration.ErrIDConflict) {
			suggestedID := app.ID + "-project"
			if customID != "" {
				suggestedID = customID + "-2"
			}
			c.Render(w, r, "migrate-modal.html", map[string]any{
				"App":         app,
				"Error":       err.Error(),
				"SuggestedID": suggestedID,
			})
			return
		}
		c.Render(w, r, "error-message.html", err)
		return
	}

	log.Printf("Migrated app %s to project %s", app.ID, project.ID)
	c.Redirect(w, r, "/project/"+project.ID)
}

func (c *AppsController) redirectToManage(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("app")
	c.Redirect(w, r, "/app/"+appID+"/manage")
}

func (c *AppsController) pollVersions(w http.ResponseWriter, r *http.Request) {
	app, err := models.Apps.Get(r.PathValue("app"))
	if err != nil {
		c.RenderError(w, r, errors.New("app not found"))
		return
	}

	c.Render(w, r, "app-versions.html", app)
}
