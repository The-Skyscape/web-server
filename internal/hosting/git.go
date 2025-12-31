package hosting

import (
	"fmt"
	"os"

	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/pkg/errors"
)

const gitRepoBasePath = "/mnt/git-repos"

// RepoPath returns the filesystem path for a repository
func RepoPath(id string) string {
	return fmt.Sprintf("%s/%s", gitRepoBasePath, id)
}

// InitGitRepo initializes a bare git repository with main as default branch
func InitGitRepo(id string) error {
	path := RepoPath(id)

	// Check if path already exists
	if _, err := os.Stat(path); err == nil {
		return errors.New("repository directory already exists")
	}

	host := containers.Local()
	if err := host.Exec("git", "init", "--bare", "--initial-branch=main", path); err != nil {
		return errors.Wrap(err, "failed to initialize git repo")
	}

	return nil
}

// RepoExists checks if a git repository already exists at the given ID
func RepoExists(id string) bool {
	_, err := os.Stat(RepoPath(id))
	return err == nil
}
