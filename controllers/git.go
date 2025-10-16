package controllers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"www.theskyscape.com/models"

	"github.com/sosedoff/gitkit"
)

func Git() (string, *GitController) {
	return "git", &GitController{}
}

type GitController struct {
	application.Controller
}

func (c *GitController) Setup(app *application.App) {
	c.Controller.Setup(app)

	http.Handle("/repo/", http.StripPrefix("/repo/", c.GitServer()))
}

func (c GitController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

// InitGitServer initializes the gitkit server with authentication
// This handles git clone, push, pull operations via HTTP
func (c *GitController) GitServer() *gitkit.Server {
	git := gitkit.New(gitkit.Config{
		Dir:        "/mnt/git-repos",
		AutoCreate: true,
		Auth:       true,
	})

	git.AuthFunc = func(creds gitkit.Credential, req *gitkit.Request) (ok bool, err error) {
		log.Println("New request:", req.Request.URL.Path)
		if creds.Username == "" || creds.Password == "" {
			return false, errors.New("authentication required")
		}

		var user *authentication.User
		if user, err = models.Auth.Users.First(`WHERE handle = ?`, creds.Username); err != nil {
			return false, errors.New("invalid username or password " + creds.Username + " " + err.Error())
		} else if !user.VerifyPassword(creds.Password) {
			return false, errors.New("invalid username or password " + creds.Password)
		} else {
			log.Printf("User auth successful for %s", creds.Username)
		}

		repo, err := models.Repos.Get(req.RepoName)
		if err != nil {
			log.Printf("Repository not found: %s", req.RepoName)
			return false, errors.New("repository not found")
		}

		isPush := strings.Contains(req.Request.URL.Path, "git-receive-pack") ||
			strings.Contains(req.Request.URL.Query().Get("service"), "git-receive-pack")

		if isPush && repo.OwnerID != user.ID {
			return false, errors.New("only owner can push to their repos")
		}

		return true, nil
	}

	if err := git.Setup(); err != nil {
		log.Fatal("Failed to setup git server: ", err)
	}

	return git
}
