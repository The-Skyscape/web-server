package models

import (
	"bytes"
	"fmt"
	"html/template"
	"path/filepath"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
	"www.theskyscape.com/internal/git"
	"www.theskyscape.com/internal/markup"
)

type Repo struct {
	application.Model
	OwnerID     string
	Name        string
	Description string
	Archived    bool
}

func (*Repo) Table() string { return "repos" }

// NewRepo creates a new repo record. Caller is responsible for:
// - Sanitizing the ID (use hosting.SanitizeID)
// - Initializing git repo (use hosting.InitGitRepo)
// - Creating the activity
func NewRepo(id, ownerID, name, description string) (*Repo, error) {
	r := &Repo{
		Model:       database.Model{ID: id},
		OwnerID:     ownerID,
		Name:        name,
		Description: description,
		Archived:    false,
	}
	return Repos.Insert(r)
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

// Stars returns all stars for this repository
func (r *Repo) Stars() []*Star {
	stars, _ := Stars.Search(`
		WHERE RepoID = ?
		ORDER BY CreatedAt DESC
	`, r.ID)
	return stars
}

// StarsCount returns the count of stars for this repository
func (r *Repo) StarsCount() int {
	return Stars.Count("WHERE RepoID = ?", r.ID)
}

// RecentStargazers returns the most recent users who starred this repository
func (r *Repo) RecentStargazers(limit int) []*Star {
	stars, _ := Stars.Search(`
		WHERE RepoID = ?
		ORDER BY CreatedAt DESC
		LIMIT ?
	`, r.ID, limit)
	return stars
}

// IsStarredBy checks if a specific user has starred this repository
func (r *Repo) IsStarredBy(userID string) bool {
	star, _ := Stars.First("WHERE UserID = ? AND RepoID = ?", userID, r.ID)
	return star != nil
}

func (r *Repo) Git(args ...string) (stdout, stderr bytes.Buffer, err error) {
	return git.Exec(r.Path(), args...)
}

func (r *Repo) ListCommits(branch string, limit int) ([]*Commit, error) {
	infos, err := git.ListCommits(r.Path(), branch, limit)
	if err != nil {
		return nil, err
	}

	var commits []*Commit
	for _, info := range infos {
		commits = append(commits, &Commit{
			Repo:    r,
			Hash:    info.Hash,
			UserID:  info.Email,
			Subject: info.Subject,
		})
	}
	return commits, nil
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
	entries, err := git.ListFiles(r.Path(), branch, path)
	if err != nil {
		return nil, err
	}

	branch = git.SanitizeBranch(branch)
	var files []*Blob
	for _, entry := range entries {
		files = append(files, &Blob{
			Repo:   r,
			Branch: branch,
			Path:   entry.Path,
			IsDir:  entry.IsDir,
		})
	}
	return files, nil
}

func (r *Repo) IsEmpty(branch string) bool {
	return git.IsEmpty(r.Path(), branch)
}

func (r *Repo) IsDir(branch, path string) (bool, error) {
	return git.IsDir(r.Path(), branch, path)
}

func (r *Repo) Open(branch, path string) (*Blob, error) {
	branch = git.SanitizeBranch(branch)
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
	fc, err := git.ReadFile(f.Repo.Path(), f.Branch, f.Path)
	if err != nil {
		return nil, err
	}

	return &Content{
		File:     f,
		Content:  fc.Content,
		IsBinary: fc.IsBinary,
	}, nil
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
	return markup.RenderMarkdown(c.Content)
}
