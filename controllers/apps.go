package controllers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
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

	app, err := models.NewApp(repo, name, description, databaseEnabled)
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
		oldID := app.ID

		// Update app ID - database will enforce uniqueness constraint
		if err := models.DB.Query("UPDATE apps SET ID = ?, Name = ?, Description = ? WHERE ID = ?", newID, name, description, oldID).Exec(); err != nil {
			// Unique constraint violation means ID is already taken
			c.Render(w, r, "error-message.html", errors.New("an app with this ID already exists"))
			return
		}

		// Update related tables with AppID column
		if err := models.DB.Query("UPDATE images SET AppID = ? WHERE AppID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[AppRename] Failed to update images.AppID from %s to %s: %v", oldID, newID, err)
		}
		if err := models.DB.Query("UPDATE app_metrics SET AppID = ? WHERE AppID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[AppRename] Failed to update app_metrics.AppID from %s to %s: %v", oldID, newID, err)
		}
		if err := models.DB.Query("UPDATE oauth_authorizations SET AppID = ? WHERE AppID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[AppRename] Failed to update oauth_authorizations.AppID from %s to %s: %v", oldID, newID, err)
		}
		if err := models.DB.Query("UPDATE oauth_authorization_codes SET ClientID = ? WHERE ClientID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[AppRename] Failed to update oauth_authorization_codes.ClientID from %s to %s: %v", oldID, newID, err)
		}

		// Update related tables with SubjectID column (for app subjects)
		if err := models.DB.Query("UPDATE activities SET SubjectID = ? WHERE SubjectType = 'app' AND SubjectID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[AppRename] Failed to update activities.SubjectID from %s to %s: %v", oldID, newID, err)
		}
		if err := models.DB.Query("UPDATE promotions SET SubjectID = ? WHERE SubjectType = 'app' AND SubjectID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[AppRename] Failed to update promotions.SubjectID from %s to %s: %v", oldID, newID, err)
		}
		if err := models.DB.Query("UPDATE comments SET SubjectID = ? WHERE SubjectID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[AppRename] Failed to update comments.SubjectID from %s to %s: %v", oldID, newID, err)
		}

		// Redirect to the new app URL
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

		if _, err := app.Build(); err != nil {
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
