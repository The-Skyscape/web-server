package models

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/pkg/errors"
)

type App struct {
	application.Model
	RepoID      string
	Name        string
	Description string
	Status      string
	Error       string
}

func (*App) Table() string { return "apps" }

func (a *App) Repo() *Repo {
	repo, err := Repos.Get(a.RepoID)
	if err != nil {
		return nil
	}

	return repo
}

func (a *App) Owner() *authentication.User {
	repo := a.Repo()
	if repo == nil {
		return nil
	}

	return repo.Owner()
}

func (app *App) Build() (*Image, error) {
	host := containers.Local()
	tmpDir, err := os.MkdirTemp("", "app-*")
	if err != nil {
		tmpDir = "/tmp/app-" + app.ID + "/" + time.Now().Format("2006-01-02-15-04-05")
		os.MkdirAll(tmpDir, os.ModePerm)
	}

	repo := app.Repo()
	if repo == nil {
		return nil, errors.New("repo not found")
	}

	var stdout, stderr bytes.Buffer
	host.SetStdout(&stdout)
	host.SetStderr(&stderr)

	if err = host.Exec("bash", "-c", fmt.Sprintf(`
		cd %[1]s
		git rev-parse --short refs/heads/main
	`, repo.Path())); err != nil {
		return nil, errors.Wrap(err, "failed to get git hash")
	}

	img, err := Images.Insert(&Image{
		AppID:   app.ID,
		Status:  "building",
		GitHash: strings.TrimSpace(stdout.String()),
	})

	log.Println("Image Tag:", img.GitHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create image")
	}

	if err = host.Exec("bash", "-c", fmt.Sprintf(`
			mkdir -p %[1]s
			git clone %[2]s %[1]s
			cd %[1]s
			git checkout main
			docker build -t %[3]s:5000/%[4]s:%[5]s .
			docker push %[3]s:5000/%[4]s:%[5]s
		`, tmpDir, repo.Path(), os.Getenv("HQ_ADDR"), app.ID, img.GitHash)); err != nil {
		img.Status = "failed"
		img.Error = err.Error()
		Images.Update(img)
		err = errors.Wrap(err, stdout.String())
		return nil, errors.Wrap(err, "failed to build image")
	}

	img.Status = "ready"
	return img, Images.Update(img)
}

func (app *App) Images() []*Image {
	images, err := Images.Search(`
		WHERE AppID = ?
		ORDER BY CreatedAt DESC
	`, app.ID)
	if err != nil {
		return nil
	}

	return images
}
