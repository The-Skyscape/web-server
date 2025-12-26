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
	// Generate ID from name, sanitizing to only allow safe characters
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

	// Initialize with starter Skykit app
	if err := p.InitStarterFiles(); err != nil {
		// Non-fatal - project still works, just empty
		fmt.Printf("warning: failed to init starter files for project %s: %v\n", p.ID, err)
	} else {
		// Trigger initial build in background
		p.Status = "launching"
		Projects.Update(p)
		go func() {
			if _, err := p.Build(); err != nil {
				fmt.Printf("warning: initial build failed for project %s: %v\n", p.ID, err)
				p.Status = "draft"
				p.Error = err.Error()
			} else {
				p.Status = "online"
				p.Error = ""
			}
			Projects.Update(p)
		}()
	}

	return p, nil
}

// InitStarterFiles creates a simple Skykit starter app in the project
func (p *Project) InitStarterFiles() error {
	// Create temp directory for working tree
	tmpDir, err := os.MkdirTemp("", "project-init-*")
	if err != nil {
		return errors.Wrap(err, "failed to create temp dir")
	}
	defer os.RemoveAll(tmpDir)

	// Initialize a new git repo (not clone - bare repo is empty)
	host := containers.Local()
	if err := host.Exec("git", "init", "--initial-branch=main", tmpDir); err != nil {
		return errors.Wrap(err, "failed to init temp repo")
	}

	// Add the bare repo as remote
	if err := host.Exec("git", "-C", tmpDir, "remote", "add", "origin", p.Path()); err != nil {
		return errors.Wrap(err, "failed to add remote")
	}

	// Create directory structure
	viewsDir := filepath.Join(tmpDir, "views")
	if err := os.MkdirAll(viewsDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create views dir")
	}

	// Write main.go - stdlib only, no external dependencies
	mainGo := fmt.Sprintf(`package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
)

//go:embed views/*
var views embed.FS

var tmpl *template.Template

func init() {
	tmpl = template.Must(template.ParseFS(views, "views/*.html"))
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]string{
			"ProjectName": "%s",
			"ProjectID":   "%s",
			"Description": "%s",
		}
		tmpl.ExecuteTemplate(w, "index.html", data)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}
	log.Printf("Server starting on port %%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
`, p.Name, p.ID, p.Description)

	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainGo), 0644); err != nil {
		return errors.Wrap(err, "failed to write main.go")
	}

	// Write go.mod - no external dependencies
	goMod := fmt.Sprintf(`module theskyscape.com/project/%s

go 1.24
`, p.ID)

	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		return errors.Wrap(err, "failed to write go.mod")
	}

	// Write Dockerfile
	dockerfile := `FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY . .
RUN go build -o app .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /build/app /app/app
COPY --from=builder /build/views /app/views
EXPOSE 5000
CMD ["/app/app"]
`

	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		return errors.Wrap(err, "failed to write Dockerfile")
	}

	// Write views/index.html - uses Go template syntax
	indexHTML := `<!DOCTYPE html>
<html data-theme="dark">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.ProjectName}}</title>
  <script src="https://cdn.jsdelivr.net/npm/htmx.org@2.0.8/dist/htmx.min.js"></script>
  <link href="https://cdn.jsdelivr.net/npm/daisyui@5" rel="stylesheet" type="text/css" />
  <link href="https://cdn.jsdelivr.net/npm/daisyui@5/themes.css" rel="stylesheet" type="text/css" />
  <script src="https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4"></script>
</head>
<body class="min-h-screen bg-base-200">
  <div class="navbar bg-base-100 shadow-lg">
    <div class="flex-1">
      <span class="text-xl font-bold px-4">{{.ProjectName}}</span>
    </div>
  </div>

  <div class="hero min-h-[calc(100vh-4rem)]">
    <div class="hero-content text-center">
      <div class="max-w-lg">
        <h1 class="text-4xl font-bold mb-4">{{.ProjectName}}</h1>
        <p class="text-lg opacity-70 mb-8">{{.Description}}</p>

        <div class="card bg-base-100 shadow-xl">
          <div class="card-body">
            <h2 class="card-title justify-center mb-4">Get Started</h2>
            <div class="mockup-code text-left text-sm">
              <pre data-prefix="$"><code>git clone https://www.theskyscape.com/project/{{.ProjectID}}</code></pre>
              <pre data-prefix="$"><code>cd {{.ProjectID}}</code></pre>
              <pre data-prefix="$"><code>git push origin main</code></pre>
            </div>
            <p class="text-sm opacity-60 mt-4">
              Push to deploy automatically. Edit <code class="text-primary">views/index.html</code>
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>
</body>
</html>
`

	if err := os.WriteFile(filepath.Join(viewsDir, "index.html"), []byte(indexHTML), 0644); err != nil {
		return errors.Wrap(err, "failed to write index.html")
	}

	// Get owner info for commit
	owner := p.Owner()
	authorName := "Skyscape"
	authorEmail := "noreply@theskyscape.com"
	if owner != nil {
		if user := owner.User(); user != nil {
			authorName = user.Name
			authorEmail = user.Email
		}
	}

	// Git add, commit, and push
	var stdout, stderr bytes.Buffer
	host.SetStdout(&stdout)
	host.SetStderr(&stderr)

	if err := host.Exec("bash", "-c", fmt.Sprintf(`
		cd %s
		git config user.name "%s"
		git config user.email "%s"
		git add -A
		git commit -m "Initial commit: Skykit starter app"
		git push origin main
	`, tmpDir, authorName, authorEmail)); err != nil {
		return errors.Wrapf(err, "failed to commit and push: %s", stderr.String())
	}

	return nil
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
