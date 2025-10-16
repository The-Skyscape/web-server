package models

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
)

func (*Repo) Table() string { return "repos" }

type Repo struct {
	application.Model
	OwnerID     string
	Name        string
	Description string
}

func NewRepo(ownerID, name, description string) (*Repo, error) {
	r := &Repo{
		Model:       database.Model{ID: strings.ReplaceAll(strings.ToLower(name), " ", "-")},
		OwnerID:     ownerID,
		Name:        name,
		Description: description,
	}

	if _, err := os.Stat(r.Path()); err == nil {
		return nil, errors.New("repo already exists")
	}

	host := containers.Local()
	if err := host.Exec("git", "init", "--bare", r.Path()); err != nil {
		return nil, errors.Wrap(err, "failed to initialize git repo")
	}

	r, err := Repos.Insert(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to insert repo")
	}

	return r, nil
}

func (r *Repo) Path() string {
	return fmt.Sprintf("/mnt/git-repos/%s", r.ID)
}

func (r *Repo) Owner() *authentication.User {
	u, err := Auth.Users.Get(r.OwnerID)
	if err != nil {
		return nil
	}

	return u
}

func (r *Repo) Git(args ...string) (stdout, stderr bytes.Buffer, err error) {
	host := containers.Local()
	host.SetStdout(&stdout)
	host.SetStderr(&stderr)
	return stdout, stderr, host.Exec(append([]string{"git"}, args...)...)

}

func (r *Repo) ListCommits(branch string, limit int) ([]*Commit, error) {
	stdout, _, err := r.Git("log", "--format=format:%H", "--reverse", fmt.Sprintf("%s..%s", branch, "HEAD"), fmt.Sprintf("--max-count=%d", limit))
	if err != nil {
		return nil, errors.Wrap(err, "failed to list commits")
	}

	commits := strings.Split(stdout.String(), "\n")
	commits = commits[:len(commits)-1]

	var commitsList []*Commit
	for _, commit := range commits {
		c := &Commit{
			Repo: r,
			Hash: commit,
		}
		commitsList = append(commitsList, c)
	}

	return commitsList, nil
}

type Commit struct {
	*Repo
	Hash    string
	Subject string
	Message string
	UserID  string
}

func (c *Commit) User() *authentication.User {
	u, err := Auth.Users.First("WHERE Handle = ?", c.UserID)
	if err != nil {
		return &authentication.User{Handle: c.UserID}
	}

	return u
}

func (r *Repo) Open(branch, path string) (*File, error) {
	stdout, _, err := r.Git("show", fmt.Sprintf("%s:%s", branch, path))
	if err != nil {
		return nil, errors.Wrap(err, "failed to show file")
	}

	file := &File{
		Repo:   r,
		Branch: branch,
		Path:   path,
	}

	if strings.Contains(file.Content, "\x00") {
		file.IsBinary = true
	} else {
		file.Content = stdout.String()
	}

	if strings.HasSuffix(file.Path, "/") {
		file.IsDir = true
	}

	return file, nil
}

type File struct {
	*Repo
	Branch   string
	Path     string
	Content  string
	IsDir    bool
	IsBinary bool
}
