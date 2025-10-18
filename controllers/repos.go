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

	http.Handle("GET /repo/{repo}", c.Serve("repo.html", auth.Optional))
	http.Handle("GET /repo/{repo}/file/{path...}", c.Serve("file.html", auth.Optional))
	http.Handle("POST /repos", c.ProtectFunc(c.createRepo, auth.Required))
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

func (c *ReposController) CurrentFile() *models.File {
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

func (c *ReposController) createRepo(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)

	name, desc := r.FormValue("name"), r.FormValue("description")
	repo, err := models.NewRepo(user.ID, name, desc)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Redirect(w, r, "/repos/"+repo.ID)
}
