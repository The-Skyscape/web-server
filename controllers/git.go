package controllers

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/sosedoff/gitkit"
	"www.theskyscape.com/models"
)

func Git() (string, *GitController) {
	return "git", &GitController{}
}

type GitController struct {
	application.Controller
}

func (c *GitController) Setup(app *application.App) {
	c.Controller.Setup(app)

	http.Handle("/repo/", http.StripPrefix("/repo/", c.gitServer()))
}

func (c GitController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

// gitServer initializes the gitkit server with authentication
// This handles git clone, push, pull operations via HTTP
func (c *GitController) gitServer() *gitkit.Server {
	git := gitkit.New(gitkit.Config{
		Dir:        "/mnt/git-repos",
		AutoCreate: true,
		Auth:       true,
	})

	git.AuthFunc = func(creds gitkit.Credential, req *gitkit.Request) (ok bool, err error) {
		isPull := strings.Contains(req.Request.URL.Path, "git-upload-pack") ||
			strings.Contains(req.Request.URL.Query().Get("service"), "git-upload-pack")

		if isPull {
			return true, nil
		}

		isPush := strings.Contains(req.Request.URL.Path, "git-receive-pack") ||
			strings.Contains(req.Request.URL.Query().Get("service"), "git-receive-pack")

		if creds.Username == "" || creds.Password == "" {
			return false, errors.New("authentication required")
		}

		var user *authentication.User
		if user, err = models.Auth.Users.First(`WHERE handle = ?`, creds.Username); err != nil {
			return false, errors.New("invalid username or password")
		} else if !user.VerifyPassword(creds.Password) {
			return false, errors.New("invalid username or password")
		} else {
			log.Printf("User auth successful for %s", creds.Username)
		}

		repo, err := models.Repos.Get(req.RepoName)
		if err != nil {
			log.Printf("Repository not found: %s", req.RepoName)
			return false, errors.New("repository not found")
		}

		if isPush && (repo.OwnerID != user.ID && !user.IsAdmin) {
			return false, errors.New("only owner can push to their repos")
		}

		// Create activity for push after successful authentication
		if isPush {
			go func(repoID, userID string) {
				// Wait for push to complete
				time.Sleep(2 * time.Second)

				// Re-fetch repo to ensure we have latest data
				repo, err := models.Repos.Get(repoID)
				if err != nil {
					return
				}

				// Get latest commit message from the repo
				stdout, _, err := repo.Git("log", "-1", "--pretty=format:%s")
				if err != nil {
					log.Printf("Failed to get commit message: %v", err)
					return
				}

				commitMsg := strings.TrimSpace(stdout.String())
				if commitMsg == "" {
					return
				}

				// Create activity
				models.Activities.Insert(&models.Activity{
					UserID:      userID,
					Action:      "pushed",
					SubjectType: "repo",
					SubjectID:   repoID,
					Content:     commitMsg,
				})
			}(repo.ID, user.ID)
		}

		return true, nil
	}

	if err := git.Setup(); err != nil {
		log.Fatal("Failed to setup git server: ", err)
	}

	return git
}
