package models

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/microcosm-cc/bluemonday"
	"github.com/pkg/errors"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

// sanitizeBranch validates and sanitizes branch names to prevent path traversal
// and unauthorized access to git refs. Returns "main" as default for invalid branches.
func sanitizeBranch(branch string) string {
	if branch == "" {
		return "main"
	}

	// Only allow alphanumeric, dash, underscore, and forward slash
	validBranchRegex := regexp.MustCompile(`^[a-zA-Z0-9/_-]+$`)
	if !validBranchRegex.MatchString(branch) {
		return "main"
	}

	// Disallow dangerous patterns that could access unauthorized refs
	dangerous := []string{
		"refs/", "HEAD~", "HEAD^", "@{",
		"..", "//", "stash",
	}

	for _, pattern := range dangerous {
		if strings.Contains(branch, pattern) {
			return "main"
		}
	}

	return branch
}

type Repo struct {
	application.Model
	OwnerID     string
	Name        string
	Description string
	Archived    bool
}

func (*Repo) Table() string { return "repos" }

func NewRepo(ownerID, name, description string) (*Repo, error) {
	id := strings.ToLower(name)
	id = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(id, "-")
	id = regexp.MustCompile(`-+`).ReplaceAllString(id, "-")
	id = strings.Trim(id, "-")

	r := &Repo{
		Model:       database.Model{ID: id},
		OwnerID:     ownerID,
		Name:        name,
		Description: description,
		Archived:    false,
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
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	return stdout, stderr, cmd.Run()
}

func (r *Repo) ListCommits(branch string, limit int) ([]*Commit, error) {
	branch = sanitizeBranch(branch)
	stdout, stderr, err := r.Git("log", "--format=format:%h %ae %s", "--reverse", branch, fmt.Sprintf("--max-count=%d", limit))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list commits: %s", stderr.String())
	}

	commits := strings.Split(stdout.String(), "\n")

	var commitsList []*Commit
	for _, commit := range commits {
		if commit == "" {
			continue
		}
		parts := strings.SplitN(commit, " ", 3)
		if len(parts) < 3 {
			continue
		}
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
	branch = sanitizeBranch(branch)
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
	branch = sanitizeBranch(branch)
	_, err := r.ListCommits(branch, 1)
	return err != nil
}

func (r *Repo) IsDir(branch, path string) (bool, error) {
	branch = sanitizeBranch(branch)
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
	branch = sanitizeBranch(branch)
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
	branch := sanitizeBranch(f.Branch)
	stdout, _, err := f.Repo.Git("show", fmt.Sprintf("%s:%s", branch, f.Path))
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
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM, // GitHub Flavored Markdown (tables, strikethrough, autolinks, task lists)
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(c.Content), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(c.Content))
	}

	p := bluemonday.UGCPolicy()
	return template.HTML(p.Sanitize(buf.String()))
}
