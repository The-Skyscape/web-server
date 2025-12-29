package hosting

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/pkg/errors"
)

// BuildResult contains the outcome of a build
type BuildResult struct {
	GitHash string
	Status  string // "ready" or "failed"
	Error   string // error message if failed
}

// Build clones, builds, and pushes a Docker image.
// Returns the git hash and status - caller is responsible for creating/updating Image records.
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
