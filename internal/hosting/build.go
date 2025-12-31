package hosting

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/pkg/errors"
	"www.theskyscape.com/models"
)

// Buildable represents an entity that can be built into a Docker image.
type Buildable interface {
	GetID() string
	RepoPath() string
	IsProject() bool
}

// appBuildable wraps an App to implement Buildable
type appBuildable struct {
	app *models.App
}

func (a *appBuildable) GetID() string    { return a.app.ID }
func (a *appBuildable) IsProject() bool  { return false }
func (a *appBuildable) RepoPath() string {
	if repo := a.app.Repo(); repo != nil {
		return repo.Path()
	}
	return ""
}

// projectBuildable wraps a Project to implement Buildable
type projectBuildable struct {
	project *models.Project
}

func (p *projectBuildable) GetID() string    { return p.project.ID }
func (p *projectBuildable) IsProject() bool  { return true }
func (p *projectBuildable) RepoPath() string { return p.project.Path() }

// BuildApp builds and pushes a Docker image for an App.
func BuildApp(app *models.App) (*models.Image, error) {
	return BuildEntity(&appBuildable{app: app})
}

// BuildProject builds and pushes a Docker image for a Project.
func BuildProject(project *models.Project) (*models.Image, error) {
	return BuildEntity(&projectBuildable{project: project})
}

// BuildEntity builds and pushes a Docker image for any Buildable entity.
// Creates Image record and updates its status.
func BuildEntity(entity Buildable) (*models.Image, error) {
	repoPath := entity.RepoPath()
	if repoPath == "" {
		return nil, errors.New("repo not found")
	}

	gitHash, err := GetGitHash(repoPath)
	if err != nil {
		return nil, err
	}

	// Create image record with appropriate ID field
	img := &models.Image{
		Status:  "building",
		GitHash: gitHash,
	}
	if entity.IsProject() {
		img.ProjectID = entity.GetID()
	} else {
		img.AppID = entity.GetID()
	}

	img, err = models.Images.Insert(img)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create image")
	}

	result, err := Build(entity.GetID(), repoPath)
	if err != nil {
		img.Status = "failed"
		img.Error = result.Error
		models.Images.Update(img)
		return nil, err
	}

	img.Status = "ready"
	return img, models.Images.Update(img)
}

// BuildResult contains the outcome of a build
type BuildResult struct {
	GitHash string
	Status  string // "ready" or "failed"
	Error   string // error message if failed
}

// Build clones, builds, and pushes a Docker image.
// Returns the git hash and status. Use BuildApp/BuildProject for full orchestration.
func Build(entityID, repoPath string) (*BuildResult, error) {
	host := containers.Local()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "build-*")
	if err != nil {
		tmpDir = fmt.Sprintf("/tmp/build-%s/%s", entityID, time.Now().Format("2006-01-02-15-04-05"))
		os.MkdirAll(tmpDir, os.ModePerm)
	}
	defer os.RemoveAll(tmpDir)

	// Get git hash
	gitHash, err := GetGitHash(repoPath)
	if err != nil {
		return nil, err
	}

	// Clone, build, and push
	var stdout, stderr bytes.Buffer
	host.SetStdout(&stdout)
	host.SetStderr(&stderr)

	hqAddr := os.Getenv("HQ_ADDR")
	buildCmd := fmt.Sprintf(`
		mkdir -p %[1]s
		git clone -b main %[2]s %[1]s
		cd %[1]s
		docker build -t %[3]s:5000/%[4]s:%[5]s .
		docker push %[3]s:5000/%[4]s:%[5]s
	`, tmpDir, repoPath, hqAddr, entityID, gitHash)

	if err = host.Exec("bash", "-c", buildCmd); err != nil {
		return &BuildResult{
			GitHash: gitHash,
			Status:  "failed",
			Error:   stderr.String(),
		}, errors.Wrap(err, "failed to build image: "+stdout.String())
	}

	return &BuildResult{
		GitHash: gitHash,
		Status:  "ready",
	}, nil
}

// GetGitHash retrieves the short hash of the main branch
func GetGitHash(repoPath string) (string, error) {
	host := containers.Local()

	var stdout, stderr bytes.Buffer
	host.SetStdout(&stdout)
	host.SetStderr(&stderr)

	if err := host.Exec("bash", "-c", fmt.Sprintf(`
		cd %s
		git rev-parse --short refs/heads/main
	`, repoPath)); err != nil {
		return "", errors.Wrap(err, "failed to get git hash")
	}

	return strings.TrimSpace(stdout.String()), nil
}
