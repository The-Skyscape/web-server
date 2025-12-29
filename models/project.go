package models

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/microcosm-cc/bluemonday"
	"github.com/pkg/errors"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"golang.org/x/crypto/bcrypt"
	"www.theskyscape.com/internal/git"
)

// Project combines code storage (like Repo) with container deployment (like App)
type Project struct {
	application.Model
	OwnerID           string
	Name              string
	Description       string
	Status            string // draft, launching, online, offline, shutdown
	Error             string
	OAuthClientSecret string // bcrypt hashed
	DatabaseEnabled   bool
}

func (*Project) Table() string { return "projects" }

// NewProject creates a new project with initialized git repo
func NewProject(ownerID, name, description string) (*Project, error) {
	// Sanitize ID - remove dangerous characters to prevent command injection
	id := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	id = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(id, "")
	id = regexp.MustCompile(`-+`).ReplaceAllString(id, "-")
	id = strings.Trim(id, "-")

	if id == "" {
		return nil, errors.New("project name must contain at least one alphanumeric character")
	}

	// Check if a project with this ID already exists
	if _, err := Projects.Get(id); err == nil {
		return nil, errors.New("a project with this ID already exists")
	}

	p := &Project{
		Model:       database.Model{ID: id},
		OwnerID:     ownerID,
		Name:        name,
		Description: description,
		Status:      "draft",
	}

	// Check if git repo path already exists
	if _, err := os.Stat(p.Path()); err == nil {
		return nil, errors.New("project directory already exists")
	}

	// Initialize bare git repo with main as default branch
	host := containers.Local()
	if err := host.Exec("git", "init", "--bare", "--initial-branch=main", p.Path()); err != nil {
		return nil, errors.Wrap(err, "failed to initialize git repo")
	}

	p, err := Projects.Insert(p)
	if err != nil {
		return nil, errors.Wrap(err, "failed to insert project")
	}

	Activities.Insert(&Activity{
		UserID:      ownerID,
		Action:      "created",
		SubjectType: "project",
		SubjectID:   p.ID,
	})

	return p, nil
}

// =============================================================================
// Ownership
// =============================================================================

func (p *Project) Owner() *Profile {
	profile, _ := Profiles.First("WHERE UserID = ?", p.OwnerID)
	return profile
}

// =============================================================================
// Git Storage
// =============================================================================

func (p *Project) Path() string {
	return fmt.Sprintf("/mnt/git-repos/%s", p.ID)
}

func (p *Project) Git(args ...string) (stdout, stderr bytes.Buffer, err error) {
	return git.Exec(p.Path(), args...)
}

func (p *Project) IsEmpty(branch string) bool {
	return git.IsEmpty(p.Path(), branch)
}

func (p *Project) ListCommits(branch string, limit int) ([]*ProjectCommit, error) {
	infos, err := git.ListCommits(p.Path(), branch, limit)
	if err != nil {
		return nil, err
	}

	var commits []*ProjectCommit
	for _, info := range infos {
		commits = append(commits, &ProjectCommit{
			Project: p,
			Hash:    info.Hash,
			UserID:  info.Email,
			Subject: info.Subject,
		})
	}
	return commits, nil
}

func (p *Project) ListFiles(branch, path string) ([]*ProjectBlob, error) {
	entries, err := git.ListFiles(p.Path(), branch, path)
	if err != nil {
		return nil, err
	}

	branch = git.SanitizeBranch(branch)
	var files []*ProjectBlob
	for _, entry := range entries {
		files = append(files, &ProjectBlob{
			Project: p,
			Branch:  branch,
			Path:    entry.Path,
			IsDir:   entry.IsDir,
		})
	}
	return files, nil
}

func (p *Project) IsDir(branch, path string) (bool, error) {
	return git.IsDir(p.Path(), branch, path)
}

func (p *Project) Open(branch, path string) (*ProjectBlob, error) {
	branch = git.SanitizeBranch(branch)
	isDir, err := p.IsDir(branch, path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read location: "+path)
	}

	return &ProjectBlob{
		Project: p,
		Branch:  branch,
		Path:    path,
		IsDir:   isDir,
	}, nil
}

// =============================================================================
// Git Types (Commit, Blob, Content)
// =============================================================================

type ProjectCommit struct {
	Project *Project
	Hash    string
	UserID  string
	Subject string
}

func (c *ProjectCommit) User() *authentication.User {
	u, err := Auth.Users.First("WHERE Handle = $1 OR Email = $1", c.UserID)
	if err != nil {
		return &authentication.User{Handle: c.UserID}
	}
	return u
}

type ProjectBlob struct {
	Project *Project
	Branch  string
	Path    string
	IsDir   bool
}

func (f *ProjectBlob) FileType() string {
	return strings.TrimPrefix(filepath.Ext(f.Path), ".")
}

func (f *ProjectBlob) Name() string {
	return filepath.Base(f.Path)
}

func (f *ProjectBlob) ListFiles(branch, _ string) ([]*ProjectBlob, error) {
	return f.Project.ListFiles(branch, f.Path)
}

func (f *ProjectBlob) Comments() ([]*Comment, error) {
	return Comments.Search(`
		WHERE SubjectID = $1
			AND Content != ''
		ORDER BY CreatedAt DESC
	`, fmt.Sprintf("file:%s:%s", f.Project.ID, f.Path))
}

func (f *ProjectBlob) Read() (*ProjectContent, error) {
	fc, err := git.ReadFile(f.Project.Path(), f.Branch, f.Path)
	if err != nil {
		return nil, err
	}

	return &ProjectContent{
		File:     f,
		Content:  fc.Content,
		IsBinary: fc.IsBinary,
	}, nil
}

type ProjectContent struct {
	File     *ProjectBlob
	Content  string
	IsBinary bool
}

func (c *ProjectContent) Lines() []string {
	return strings.Split(c.Content, "\n")
}

func (c *ProjectContent) Markdown() template.HTML {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(c.Content), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(c.Content))
	}

	policy := bluemonday.UGCPolicy()
	return template.HTML(policy.Sanitize(buf.String()))
}

// =============================================================================
// Stars
// =============================================================================

func (p *Project) Stars() []*Star {
	stars, _ := Stars.Search(`
		WHERE ProjectID = ?
		ORDER BY CreatedAt DESC
	`, p.ID)
	return stars
}

func (p *Project) StarsCount() int {
	return Stars.Count("WHERE ProjectID = ?", p.ID)
}

func (p *Project) RecentStargazers(limit int) []*Star {
	stars, _ := Stars.Search(`
		WHERE ProjectID = ?
		ORDER BY CreatedAt DESC
		LIMIT ?
	`, p.ID, limit)
	return stars
}

func (p *Project) IsStarredBy(userID string) bool {
	star, _ := Stars.First("WHERE UserID = ? AND ProjectID = ?", userID, p.ID)
	return star != nil
}

// =============================================================================
// Images & Deployment
// =============================================================================

func (p *Project) Images() []*Image {
	images, _ := Images.Search(`
		WHERE ProjectID = ?
		ORDER BY CreatedAt DESC
	`, p.ID)
	return images
}

func (p *Project) ActiveImage() *Image {
	img, _ := Images.First(`
		WHERE ProjectID = ? AND Status = 'running'
		ORDER BY CreatedAt DESC
	`, p.ID)
	return img
}

func (p *Project) Build() (*Image, error) {
	host := containers.Local()
	tmpDir, err := os.MkdirTemp("", "project-*")
	if err != nil {
		tmpDir = "/tmp/project-" + p.ID + "/" + time.Now().Format("2006-01-02-15-04-05")
		os.MkdirAll(tmpDir, os.ModePerm)
	}
	defer os.RemoveAll(tmpDir)

	var stdout, stderr bytes.Buffer
	host.SetStdout(&stdout)
	host.SetStderr(&stderr)

	// Get git hash
	if err = host.Exec("bash", "-c", fmt.Sprintf(`
		cd %[1]s
		git rev-parse --short refs/heads/main
	`, p.Path())); err != nil {
		return nil, errors.Wrap(err, "failed to get git hash")
	}

	img, err := Images.Insert(&Image{
		ProjectID: p.ID,
		Status:    "building",
		GitHash:   strings.TrimSpace(stdout.String()),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create image")
	}

	stdout.Reset()
	stderr.Reset()

	// Clone, build, and push
	if err = host.Exec("bash", "-c", fmt.Sprintf(`
		mkdir -p %[1]s
		git clone -b main %[2]s %[1]s
		cd %[1]s
		docker build -t %[3]s:5000/%[4]s:%[5]s .
		docker push %[3]s:5000/%[4]s:%[5]s
	`, tmpDir, p.Path(), os.Getenv("HQ_ADDR"), p.ID, img.GitHash)); err != nil {
		img.Status = "failed"
		img.Error = stderr.String()
		Images.Update(img)
		return nil, errors.Wrap(err, "failed to build image: "+stdout.String())
	}

	img.Status = "ready"
	return img, Images.Update(img)
}

// =============================================================================
// OAuth
// =============================================================================

func (p *Project) RedirectURI() string {
	return fmt.Sprintf("https://%s.skysca.pe/auth/callback", p.ID)
}

func (p *Project) AllowedScopes() string {
	return "user:read"
}

func (p *Project) VerifySecret(secret string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(p.OAuthClientSecret), []byte(secret))
	return err == nil
}

func (p *Project) AuthorizedUsers() []*OAuthAuthorization {
	auths, _ := OAuthAuthorizations.Search(`
		WHERE ProjectID = ? AND Revoked = false
	`, p.ID)
	return auths
}

func (p *Project) AuthorizedUsersCount() int {
	return OAuthAuthorizations.Count("WHERE ProjectID = ? AND Revoked = false", p.ID)
}

// =============================================================================
// Comments & Promotions
// =============================================================================

func (p *Project) Comments(limit, offset int) []*Comment {
	comments, _ := Comments.Search(`
		WHERE SubjectID = ?
			AND Content != ''
		ORDER BY CreatedAt DESC
		LIMIT ? OFFSET ?
	`, p.ID, limit, offset)
	return comments
}

func (p *Project) ActivePromotion() *Promotion {
	promo, _ := Promotions.First(`
		WHERE SubjectType = 'project' AND SubjectID = ? AND ExpiresAt > ?
		ORDER BY CreatedAt DESC
	`, p.ID, time.Now())
	return promo
}

// =============================================================================
// Metrics
// =============================================================================

func (p *Project) Metrics() *AppMetrics {
	metrics, _ := AppMetricsManager.First("WHERE ProjectID = ?", p.ID)
	return metrics
}
