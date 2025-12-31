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
	"www.theskyscape.com/internal/hosting"
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

	http.Handle("/repo/", http.StripPrefix("/repo/", c.repoGitServer()))
	http.Handle("/project/", http.StripPrefix("/project/", c.projectGitServer()))
}

func (c GitController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

// repoGitServer initializes the gitkit server for repos with authentication
// This handles git clone, push, pull operations via HTTP for legacy repos
func (c *GitController) repoGitServer() *gitkit.Server {
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

		// Check if this is a push operation (either refs discovery or actual push)
		isPushService := strings.Contains(req.Request.URL.Query().Get("service"), "git-receive-pack")
		isPushPack := strings.Contains(req.Request.URL.Path, "git-receive-pack")
		isPush := isPushService || isPushPack

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

		// Create activity and trigger auto-deploy only on actual pack upload (not refs discovery)
		if isPushPack {
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

				// Auto-deploy: trigger build for any apps linked to this repo
				apps, err := repo.Apps()
				if err != nil || len(apps) == 0 {
					return
				}

				for _, app := range apps {
					// Skip shutdown apps
					if app.Status == "shutdown" {
						continue
					}

					log.Printf("[AutoDeploy] Triggering build for app %s after push to %s", app.ID, repoID)

					// Start build in background
					go func(a *models.App) {
						a.Status = "launching"
						a.Error = ""
						models.Apps.Update(a)

						if _, err := hosting.BuildApp(a); err != nil {
							a.Error = err.Error()
							models.Apps.Update(a)
							log.Printf("[AutoDeploy] Build failed for app %s: %v", a.ID, err)
						}
					}(app)
				}
			}(repo.ID, user.ID)
		}

		return true, nil
	}

	if err := git.Setup(); err != nil {
		log.Fatal("Failed to setup git server: ", err)
	}

	return git
}

// projectGitServer initializes the gitkit server for projects with authentication
// This handles git clone, push, pull operations via HTTP for projects
// Push triggers auto-deploy directly (no apps indirection)
func (c *GitController) projectGitServer() *gitkit.Server {
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

		// Check if this is a push operation (either refs discovery or actual push)
		isPushService := strings.Contains(req.Request.URL.Query().Get("service"), "git-receive-pack")
		isPushPack := strings.Contains(req.Request.URL.Path, "git-receive-pack")
		isPush := isPushService || isPushPack

		if creds.Username == "" || creds.Password == "" {
			return false, errors.New("authentication required")
		}

		var user *authentication.User
		if user, err = models.Auth.Users.First(`WHERE handle = ?`, creds.Username); err != nil {
			return false, errors.New("invalid username or password")
		} else if !user.VerifyPassword(creds.Password) {
			return false, errors.New("invalid username or password")
		} else {
			log.Printf("User auth successful for %s (project)", creds.Username)
		}

		project, err := models.Projects.Get(req.RepoName)
		if err != nil {
			log.Printf("Project not found: %s", req.RepoName)
			return false, errors.New("project not found")
		}

		if isPush && (project.OwnerID != user.ID && !user.IsAdmin) {
			return false, errors.New("only owner can push to their projects")
		}

		// Create activity and trigger auto-deploy only on actual pack upload (not refs discovery)
		if isPushPack {
			go func(projectID, userID string) {
				// Wait for push to complete
				time.Sleep(2 * time.Second)

				// Re-fetch project to ensure we have latest data
				project, err := models.Projects.Get(projectID)
				if err != nil {
					return
				}

				// Get latest commit message from the project
				stdout, _, err := project.Git("log", "-1", "--pretty=format:%s")
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
					SubjectType: "project",
					SubjectID:   projectID,
					Content:     commitMsg,
				})

				// Auto-deploy: trigger build for the project directly
				if project.Status == "shutdown" {
					return
				}

				log.Printf("[AutoDeploy] Triggering build for project %s after push", projectID)

				project.Status = "launching"
				project.Error = ""
				models.Projects.Update(project)

				if _, err := hosting.BuildProject(project); err != nil {
					project.Error = err.Error()
					models.Projects.Update(project)
					log.Printf("[AutoDeploy] Build failed for project %s: %v", projectID, err)
				}
			}(project.ID, user.ID)
		}

		return true, nil
	}

	if err := git.Setup(); err != nil {
		log.Fatal("Failed to setup project git server: ", err)
	}

	return git
}
