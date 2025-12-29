package starter

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"text/template"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/pkg/errors"
	"www.theskyscape.com/models"
)

//go:embed templates/*
var templates embed.FS

// CreateStarterFiles creates a Skykit starter app in the project repository
func CreateStarterFiles(repoPath string, project *models.Project, author *authentication.User) error {
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
	if err := host.Exec("git", "-C", tmpDir, "remote", "add", "origin", repoPath); err != nil {
		return errors.Wrap(err, "failed to add remote")
	}

	// Create directory structure
	viewsDir := filepath.Join(tmpDir, "views")
	if err := os.MkdirAll(viewsDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create views dir")
	}

	// Generate and write files from templates
	if err := writeTemplate(tmpDir, "main.go", "templates/main.go.tmpl", project); err != nil {
		return err
	}
	if err := writeTemplate(tmpDir, "go.mod", "templates/go.mod.tmpl", project); err != nil {
		return err
	}
	if err := writeStatic(tmpDir, "Dockerfile", "templates/Dockerfile"); err != nil {
		return err
	}
	if err := writeTemplate(viewsDir, "index.html", "templates/views/index.html.tmpl", project); err != nil {
		return err
	}

	// Git add, commit, and push
	var stdout, stderr bytes.Buffer
	host.SetStdout(&stdout)
	host.SetStderr(&stderr)

	if err := host.Exec("bash", "-c", buildCommitScript(tmpDir, author)); err != nil {
		return errors.Wrapf(err, "failed to commit and push: %s", stderr.String())
	}

	return nil
}

func writeTemplate(dir, filename, tmplPath string, data *models.Project) error {
	content, err := templates.ReadFile(tmplPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read template %s", tmplPath)
	}

	tmpl, err := template.New(filename).Parse(string(content))
	if err != nil {
		return errors.Wrapf(err, "failed to parse template %s", tmplPath)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return errors.Wrapf(err, "failed to execute template %s", tmplPath)
	}

	if err := os.WriteFile(filepath.Join(dir, filename), buf.Bytes(), 0644); err != nil {
		return errors.Wrapf(err, "failed to write %s", filename)
	}

	return nil
}

func writeStatic(dir, filename, srcPath string) error {
	content, err := templates.ReadFile(srcPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", srcPath)
	}

	if err := os.WriteFile(filepath.Join(dir, filename), content, 0644); err != nil {
		return errors.Wrapf(err, "failed to write %s", filename)
	}

	return nil
}

func buildCommitScript(tmpDir string, user *authentication.User) string {
	return `
		cd ` + tmpDir + `
		git config user.name "` + user.Name + `"
		git config user.email "` + user.Email + `"
		git add -A
		git commit -m "Initial commit: Skykit starter app"
		git push origin main
	`
}
