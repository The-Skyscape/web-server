package models

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
	"github.com/yuin/goldmark"
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

	Activities.Insert(&Activity{
		UserID:      ownerID,
		Action:      "created",
		SubjectType: "repo",
		SubjectID:   r.ID,
	})

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

func (r *Repo) Comments() ([]*Comment, error) {
	return Comments.Search(`
		WHERE SubjectID = $1
			AND Content != ''
		ORDER BY CreatedAt DESC
	`, r.ID)
}

func (r *Repo) Apps() ([]*App, error) {
	return Apps.Search(`
		WHERE RepoID = $1
		ORDER BY CreatedAt DESC
	`, r.ID)
}

func (r *Repo) Git(args ...string) (stdout, stderr bytes.Buffer, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	return stdout, stderr, cmd.Run()
}

func (r *Repo) ListCommits(branch string, limit int) ([]*Commit, error) {
	stdout, stderr, err := r.Git("log", "--format=format:%h %ae %s", "--reverse", branch, fmt.Sprintf("--max-count=%d", limit))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list commits: %s", stderr.String())
	}

	commits := strings.Split(stdout.String(), "\n")
	log.Println("Commits:", commits)

	var commitsList []*Commit
	for _, commit := range commits {
		log.Println("Commit whole:", commit)
		parts := strings.SplitN(commit, " ", 3)
		log.Println("Commit parts:", parts)
		c := &Commit{
			Repo:    r,
			Hash:    parts[0],
			UserID:  parts[1],
			Subject: parts[2],
		}
		commitsList = append(commitsList, c)
	}

	return commitsList, nil
}

type Commit struct {
	Repo    *Repo
	Hash    string
	UserID  string
	Subject string
}

func (c *Commit) User() *authentication.User {
	u, err := Auth.Users.First("WHERE Handle = $1 OR Email = $1", c.UserID)
	if err != nil {
		return &authentication.User{Handle: c.UserID}
	}

	return u
}

func (r *Repo) ListFiles(branch, path string) ([]*Blob, error) {
	stdout, _, err := r.Git("ls-tree", branch, filepath.Join(".", path)+"/")
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to list files: %s @ %s", branch, path)
	}

	var files []*Blob
	for line := range strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n") {
		if parts := strings.Fields(line); len(parts) >= 4 {
			files = append(files, &Blob{
				Repo:   r,
				Branch: branch,
				Path:   parts[3],
				IsDir:  parts[1] == "tree",
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir && !files[j].IsDir {
			return true
		}
		if !files[i].IsDir && files[j].IsDir {
			return false
		}
		return files[i].Path < files[j].Path
	})

	return files, nil
}

func (r *Repo) IsEmpty(branch string) bool {
	_, err := r.ListCommits(branch, 1)
	return err != nil
}

func (r *Repo) IsDir(branch, path string) (bool, error) {
	if path == "" || path == "." {
		return true, nil
	}

	stdout, _, err := r.Git("ls-tree", branch, filepath.Join(".", path))
	if err != nil {
		return false, errors.Wrap(err, "failed to list files")
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return false, errors.New("no such file or directory")
	}

	parts := strings.Fields(output)
	return parts[1] == "tree", nil
}

func (r *Repo) Open(branch, path string) (*Blob, error) {
	isDir, err := r.IsDir(branch, path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read location: "+path)
	}

	return &Blob{
		Repo:   r,
		Branch: branch,
		Path:   path,
		IsDir:  isDir,
	}, nil
}

type Blob struct {
	Repo   *Repo
	Branch string
	Path   string
	IsDir  bool
}

func (f *Blob) FileType() (ext string) {
	return strings.TrimPrefix(filepath.Ext(f.Path), ".")
}

func (f *Blob) Name() string {
	return filepath.Base(f.Path)
}

func (f *Blob) ListFiles(branch, _ string) ([]*Blob, error) {
	return f.Repo.ListFiles(branch, f.Path)
}

func (f *Blob) Comments() ([]*Comment, error) {
	return Comments.Search(`
		WHERE SubjectID = $1
			AND Content != ''
		ORDER BY CreatedAt DESC
	`, fmt.Sprintf("file:%s:%s", f.Repo.ID, f.Path))
}

func (f *Blob) Read() (*Content, error) {
	stdout, _, err := f.Repo.Git("show", fmt.Sprintf("%s:%s", f.Branch, f.Path))
	if err != nil {
		return nil, errors.Wrap(err, "failed to show file")
	}

	c := &Content{
		File:    f,
		Content: stdout.String(),
	}

	if strings.Contains(c.Content, "\x00") {
		c.IsBinary = true
	}

	return c, nil
}

type Content struct {
	File     *Blob
	Content  string
	IsBinary bool
}

func (c *Content) Lines() []string {
	return strings.Split(c.Content, "\n")
}

func (c *Content) Markdown() template.HTML {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(c.Content), &buf); err != nil {
		return template.HTML(c.Content)
	}

	return template.HTML(buf.String())
}
