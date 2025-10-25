package controllers

import (
	"cmp"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/models"
)

func Repos() (string, *ReposController) {
	return "repos", &ReposController{}
}

type ReposController struct {
	application.Controller
}

func (c *ReposController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := app.Use("auth").(*AuthController)

	http.Handle("GET /repos", c.Serve("repos.html", auth.Optional))
	http.Handle("GET /repo/{repo}", c.Serve("repo.html", auth.Optional))
	http.Handle("GET /repo/{repo}/file/{path...}", c.Serve("file.html", auth.Optional))
	http.Handle("POST /repos", c.ProtectFunc(c.createRepo, auth.Required))
	http.Handle("POST /repos/{repo}/comments", c.ProtectFunc(c.comment, auth.Required))
}

func (c ReposController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *ReposController) CurrentRepo() *models.Repo {
	repo, err := models.Repos.Get(c.PathValue("repo"))
	if err != nil {
		return nil
	}

	return repo
}

func (c *ReposController) AllRepos() []*models.Repo {
	query := c.URL.Query().Get("query")
	repos, _ := models.Repos.Search(`
	  INNER JOIN users on users.ID = repos.OwnerID
		WHERE 
			repos.Name           LIKE $1        OR
			repos.Description    LIKE $1        OR
			users.Handle         LIKE LOWER($1)
		ORDER BY repos.CreatedAt DESC
	`, "%"+query+"%")
	return repos
}

func (c *ReposController) RecentRepos() []*models.Repo {
	query := c.URL.Query().Get("query")
	repos, _ := models.Repos.Search(`
	  INNER JOIN users on users.ID = repos.OwnerID
		WHERE 
			repos.Name           LIKE $1        OR
			repos.Description    LIKE $1        OR
			users.Handle         LIKE LOWER($1)
		ORDER BY repos.CreatedAt DESC
		LIMIT 4
	`, "%"+query+"%")
	return repos
}

func (c *ReposController) CurrentFile() *models.Blob {
	repo := c.CurrentRepo()
	if repo == nil {
		return nil
	}

	branch := cmp.Or(c.URL.Query().Get("branch"), "main")
	path := c.PathValue("path")
	if file, err := repo.Open(branch, path); err == nil {
		return file
	}

	return nil
}

func (c *ReposController) LatestCommit() *models.Commit {
	repo := c.CurrentRepo()
	if repo == nil {
		return nil
	}

	branch := cmp.Or(c.URL.Query().Get("branch"), "main")
	commits, err := repo.ListCommits(branch, 1)
	if err != nil {
		return nil
	}

	return commits[0]
}

func (c *ReposController) FilePath() []PathPart {
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

type PathPart struct {
	Href, Label string
}

func (c *ReposController) ReadmeFile() *models.Blob {
	repo := c.CurrentRepo()
	if repo == nil {
		return nil
	}

	branch := cmp.Or(c.URL.Query().Get("branch"), "main")
	files := []string{"README.md", "README", "readme.md", "readme"}

	for _, name := range files {
		if file, err := repo.Open(branch, name); err == nil {
			return file
		}
	}

	return nil
}

func (c *ReposController) createRepo(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)

	name, desc := r.FormValue("name"), r.FormValue("description")
	repo, err := models.NewRepo(user.ID, name, desc)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Redirect(w, r, "/repo/"+repo.ID)
}

func (c *ReposController) comment(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	if _, err = models.Comments.Insert(&models.Comment{
		UserID:    user.ID,
		SubjectID: r.PathValue("repo"),
		Content:   r.FormValue("content"),
	}); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}
