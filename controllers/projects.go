package controllers

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/models"
)

func Projects() (string, *ProjectsController) {
	return "projects", &ProjectsController{}
}

type ProjectsController struct {
	application.Controller
}

func (c *ProjectsController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := app.Use("auth").(*AuthController)

	http.Handle("GET /projects", c.Serve("projects.html", auth.Optional))
	http.Handle("GET /project/{project}", c.Serve("project.html", auth.Optional))
	http.Handle("GET /project/{project}/manage", c.Serve("project-manage.html", auth.Required))
	http.Handle("GET /project/{project}/file/{path...}", c.Serve("project-file.html", auth.Optional))
	http.Handle("GET /project/{project}/comments", c.Serve("project-comments.html", auth.Optional))
	http.Handle("GET /project/{project}/versions", c.ProtectFunc(c.pollVersions, auth.Required))
	http.Handle("POST /projects", c.ProtectFunc(c.create, auth.Required))
	http.Handle("POST /project/{project}/edit", c.ProtectFunc(c.update, auth.Required))
	http.Handle("POST /project/{project}/launch", c.ProtectFunc(c.launch, auth.Required))
	http.Handle("POST /project/{project}/enable-database", c.ProtectFunc(c.enableDatabase, auth.Required))
	http.Handle("POST /project/{project}/star", c.ProtectFunc(c.toggleStar, auth.Required))
	http.Handle("POST /project/{project}/share", c.ProtectFunc(c.shareProject, auth.Required))
	http.Handle("POST /projects/{project}/promote", c.ProtectFunc(c.promoteProject, auth.Required))
	http.Handle("DELETE /projects/{project}/promote", c.ProtectFunc(c.cancelPromotion, auth.Required))
	http.Handle("DELETE /project/{project}", c.ProtectFunc(c.shutdown, auth.Required))
}

func (c ProjectsController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

// =============================================================================
// Template Methods
// =============================================================================

func (c *ProjectsController) CurrentProject() *models.Project {
	project, err := models.Projects.Get(c.PathValue("project"))
	if err != nil {
		return nil
	}
	return project
}

func (c *ProjectsController) MyProjects() []*models.Project {
	auth := c.Use("auth").(*AuthController)
	user := auth.CurrentUser()
	if user == nil {
		return nil
	}

	projects, _ := models.Projects.Search(`
		WHERE OwnerID = ? AND Status != 'shutdown'
		ORDER BY CreatedAt DESC
	`, user.ID)
	return projects
}

func (c *ProjectsController) AllProjects() []*models.Project {
	query := c.URL.Query().Get("query")
	projects, _ := models.Projects.Search(`
		INNER JOIN users ON users.ID = projects.OwnerID
		WHERE
			projects.Status != 'shutdown'
			AND (
				projects.Name        LIKE $1 OR
				projects.Description LIKE $1 OR
				users.Handle         LIKE LOWER($1)
			)
		ORDER BY projects.CreatedAt DESC
	`, "%"+query+"%")
	return projects
}

func (c *ProjectsController) RecentProjects() []*models.Project {
	query := c.URL.Query().Get("query")
	projects, _ := models.Projects.Search(`
		INNER JOIN users ON users.ID = projects.OwnerID
		WHERE
			projects.Status != 'shutdown'
			AND (
				projects.Name        LIKE $1 OR
				projects.Description LIKE $1 OR
				users.Handle         LIKE LOWER($1)
			)
		ORDER BY (SELECT COUNT(*) FROM stars WHERE ProjectID = projects.ID) DESC
		LIMIT 3
	`, "%"+query+"%")
	return projects
}

func (c *ProjectsController) CurrentFile() *models.ProjectBlob {
	project := c.CurrentProject()
	if project == nil {
		return nil
	}

	branch := cmp.Or(c.URL.Query().Get("branch"), "main")
	path := c.PathValue("path")
	if file, err := project.Open(branch, path); err == nil {
		return file
	}

	return nil
}

func (c *ProjectsController) LatestCommit() *models.ProjectCommit {
	project := c.CurrentProject()
	if project == nil {
		return nil
	}

	branch := cmp.Or(c.URL.Query().Get("branch"), "main")
	commits, err := project.ListCommits(branch, 1)
	if err != nil || len(commits) == 0 {
		return nil
	}

	return commits[0]
}

func (c *ProjectsController) FilePath() []PathPart {
	path := c.PathValue("path")
	if path == "" {
		return []PathPart{
			{Href: "", Label: "."},
		}
	}

	if file := c.CurrentFile(); file != nil && !file.IsDir {
		path = filepath.Dir(path)
	}

	if path[0] != '.' {
		path = fmt.Sprintf("./%s", path)
	}

	parts, res := []string{}, []PathPart{}
	for part := range strings.SplitSeq(path, "/") {
		parts = append(parts, part)
		res = append(res, PathPart{
			Href:  filepath.Join(parts...),
			Label: part,
		})
	}

	return res
}

func (c *ProjectsController) ReadmeFile() *models.ProjectBlob {
	project := c.CurrentProject()
	if project == nil {
		return nil
	}

	branch := cmp.Or(c.URL.Query().Get("branch"), "main")
	files := []string{"README.md", "README", "readme.md", "readme"}

	for _, name := range files {
		if file, err := project.Open(branch, name); err == nil {
			return file
		}
	}

	return nil
}

func (c *ProjectsController) CurrentProjectMetrics() *models.AppMetrics {
	project := c.CurrentProject()
	if project == nil {
		return nil
	}

	metrics, err := models.AppMetricsManager.First("WHERE ProjectID = ?", project.ID)
	if err != nil {
		return nil
	}

	return metrics
}

// Comment pagination
const defaultProjectCommentLimit = 10

func (c *ProjectsController) CommentPage() int {
	page, _ := strconv.Atoi(c.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	return page
}

func (c *ProjectsController) CommentLimit() int {
	limit, _ := strconv.Atoi(c.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = defaultProjectCommentLimit
	}
	return limit
}

func (c *ProjectsController) CommentNextPage() int {
	return c.CommentPage() + 1
}

func (c *ProjectsController) Comments() []*models.Comment {
	project := c.CurrentProject()
	if project == nil {
		return nil
	}
	limit := c.CommentLimit()
	offset := (c.CommentPage() - 1) * limit
	return project.Comments(limit, offset)
}

func (c *ProjectsController) AuthorizedUsers() []*models.OAuthAuthorization {
	project := c.CurrentProject()
	if project == nil {
		return nil
	}

	auths, _ := models.OAuthAuthorizations.Search(`
		WHERE ProjectID = ?
		AND Revoked = false
	`, project.ID)
	return auths
}

// =============================================================================
// Handlers
// =============================================================================

func (c *ProjectsController) create(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))

	if name == "" || description == "" {
		c.Render(w, r, "error-message.html", errors.New("name and description are required"))
		return
	}

	project, err := models.NewProject(user.ID, name, description)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Enable database if requested
	if r.FormValue("database") == "true" {
		project.DatabaseEnabled = true
		models.Projects.Update(project)
	}

	c.Redirect(w, r, "/project/"+project.ID)
}

func (c *ProjectsController) update(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	project, err := models.Projects.Get(r.PathValue("project"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("project not found"))
		return
	}

	if project.OwnerID != user.ID && !user.IsAdmin {
		c.Render(w, r, "error-message.html", errors.New("you are not the owner"))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))

	if name == "" || description == "" {
		c.Render(w, r, "error-message.html", errors.New("name and description are required"))
		return
	}

	project.Name = name
	project.Description = description

	// Handle ID change (admin only)
	newID := r.FormValue("id")
	if newID != "" && newID != project.ID && user.IsAdmin {
		oldID := project.ID

		// Update project ID - database will enforce uniqueness constraint
		if err := models.DB.Query("UPDATE projects SET ID = ?, Name = ?, Description = ? WHERE ID = ?", newID, name, description, oldID).Exec(); err != nil {
			c.Render(w, r, "error-message.html", errors.New("a project with this ID already exists"))
			return
		}

		// Update related tables with ProjectID column
		if err := models.DB.Query("UPDATE images SET ProjectID = ? WHERE ProjectID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[ProjectRename] Failed to update images.ProjectID from %s to %s: %v", oldID, newID, err)
		}
		if err := models.DB.Query("UPDATE app_metrics SET ProjectID = ? WHERE ProjectID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[ProjectRename] Failed to update app_metrics.ProjectID from %s to %s: %v", oldID, newID, err)
		}
		if err := models.DB.Query("UPDATE oauth_authorizations SET ProjectID = ? WHERE ProjectID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[ProjectRename] Failed to update oauth_authorizations.ProjectID from %s to %s: %v", oldID, newID, err)
		}
		if err := models.DB.Query("UPDATE stars SET ProjectID = ? WHERE ProjectID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[ProjectRename] Failed to update stars.ProjectID from %s to %s: %v", oldID, newID, err)
		}

		// Update related tables with SubjectID column (for project subjects)
		if err := models.DB.Query("UPDATE activities SET SubjectID = ? WHERE SubjectType = 'project' AND SubjectID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[ProjectRename] Failed to update activities.SubjectID from %s to %s: %v", oldID, newID, err)
		}
		if err := models.DB.Query("UPDATE promotions SET SubjectID = ? WHERE SubjectType = 'project' AND SubjectID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[ProjectRename] Failed to update promotions.SubjectID from %s to %s: %v", oldID, newID, err)
		}
		if err := models.DB.Query("UPDATE comments SET SubjectID = ? WHERE SubjectID = ?", newID, oldID).Exec(); err != nil {
			log.Printf("[ProjectRename] Failed to update comments.SubjectID from %s to %s: %v", oldID, newID, err)
		}

		c.Redirect(w, r, "/project/"+newID+"/manage")
		return
	}

	if err := models.Projects.Update(project); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}

func (c *ProjectsController) launch(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	project, err := models.Projects.Get(r.PathValue("project"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("project not found"))
		return
	}

	if project.OwnerID != user.ID && !user.IsAdmin {
		c.Render(w, r, "error-message.html", errors.New("permission denied"))
		return
	}

	go func() {
		project.Status = "launching"
		project.Error = ""
		models.Projects.Update(project)

		if _, err := project.Build(); err != nil {
			project.Status = "draft"
			project.Error = err.Error()
			models.Projects.Update(project)
			return
		}

		project.Status = "online"
		project.Error = ""
		models.Projects.Update(project)
	}()

	time.Sleep(time.Millisecond * 250)
	c.Refresh(w, r)
}

func (c *ProjectsController) enableDatabase(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	project, err := models.Projects.Get(r.PathValue("project"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("project not found"))
		return
	}

	if project.OwnerID != user.ID && !user.IsAdmin {
		c.Render(w, r, "error-message.html", errors.New("permission denied"))
		return
	}

	if project.DatabaseEnabled {
		c.Render(w, r, "error-message.html", errors.New("database already enabled"))
		return
	}

	project.DatabaseEnabled = true
	models.Projects.Update(project)

	go func() {
		project.Status = "launching"
		project.Error = ""
		models.Projects.Update(project)

		if _, err := project.Build(); err != nil {
			project.Error = err.Error()
			models.Projects.Update(project)
			return
		}
	}()

	time.Sleep(time.Millisecond * 250)
	c.Refresh(w, r)
}

func (c *ProjectsController) toggleStar(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	project, err := models.Projects.Get(r.PathValue("project"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("project not found"))
		return
	}

	// Check if already starred
	star, _ := models.Stars.First("WHERE UserID = ? AND ProjectID = ?", user.ID, project.ID)
	if star != nil {
		// Unstar
		if err := models.Stars.Delete(star); err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}
	} else {
		// Star
		if _, err := models.Stars.Insert(&models.Star{
			UserID:    user.ID,
			ProjectID: project.ID,
		}); err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}
	}

	c.Refresh(w, r)
}

func (c *ProjectsController) shareProject(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	project, err := models.Projects.Get(r.PathValue("project"))
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
		SubjectType: "project",
		SubjectID:   project.ID,
		Content:     content,
	}); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Redirect(w, r, "/")
}

func (c *ProjectsController) promoteProject(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	project, err := models.Projects.Get(r.PathValue("project"))
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	if project.OwnerID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("you can only promote your own projects"))
		return
	}

	if existing := project.ActivePromotion(); existing != nil {
		c.Render(w, r, "error-message.html", errors.New("this project already has an active promotion"))
		return
	}

	content := r.FormValue("content")
	if len(content) > MaxContentLength {
		c.Render(w, r, "error-message.html", errors.New("promotion content too long"))
		return
	}

	if _, err = models.Promotions.Insert(&models.Promotion{
		UserID:      user.ID,
		SubjectType: "project",
		SubjectID:   project.ID,
		Content:     content,
		ExpiresAt:   time.Now().Add(models.DefaultPromotionDuration),
	}); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Redirect(w, r, "/")
}

func (c *ProjectsController) cancelPromotion(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	project, err := models.Projects.Get(r.PathValue("project"))
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	if project.OwnerID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("you can only cancel your own promotions"))
		return
	}

	promo := project.ActivePromotion()
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

func (c *ProjectsController) shutdown(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	project, err := models.Projects.Get(r.PathValue("project"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("project not found"))
		return
	}

	if project.OwnerID != user.ID && !user.IsAdmin {
		c.Render(w, r, "error-message.html", errors.New("permission denied"))
		return
	}

	project.Status = "shutdown"
	if err = models.Projects.Update(project); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Redirect(w, r, "/profile")
}

func (c *ProjectsController) pollVersions(w http.ResponseWriter, r *http.Request) {
	project, err := models.Projects.Get(r.PathValue("project"))
	if err != nil {
		c.RenderError(w, r, errors.New("project not found"))
		return
	}

	c.Render(w, r, "project-versions.html", project)
}
