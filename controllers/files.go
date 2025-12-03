package controllers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/pkg/errors"
	"www.theskyscape.com/models"
)

func Files() (string, *FilesController) {
	return "files", &FilesController{}
}

type FilesController struct {
	application.Controller
}

func (c *FilesController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := c.Use("auth").(*AuthController)

	http.Handle("GET /files", c.Serve("files.html", auth.Required))
	http.Handle("POST /files", c.ProtectFunc(c.uploadFile, auth.Required))
	http.Handle("GET /file/{file}", c.ProtectFunc(c.serveFile, auth.Optional))
}

func (c FilesController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *FilesController) MyFiles() []*models.File {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil
	}

	files, _ := models.Files.Search(`
		WHERE OwnerID = ?
		ORDER BY CreatedAt DESC
	`, user.ID)
	return files
}

const maxFileSize = 10 * 1024 * 1024 // 10MB

var allowedMimeTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/gif":       true,
	"image/webp":      true,
	"application/pdf": true,
	"text/plain":      true,
	"text/markdown":   true,
}

func (c *FilesController) uploadFile(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	r.ParseMultipartForm(maxFileSize)
	file, handler, err := r.FormFile("file")
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	defer file.Close()

	// Validate file size
	if handler.Size > maxFileSize {
		c.Render(w, r, "error-message.html", errors.New("file too large, max 10MB"))
		return
	}

	// Validate MIME type
	mimeType := handler.Header.Get("Content-Type")
	if !allowedMimeTypes[mimeType] {
		c.Render(w, r, "error-message.html", errors.New("file type not allowed"))
		return
	}

	// Sanitize filename to prevent path traversal
	filename := filepath.Base(filepath.Clean(handler.Filename))
	if filename == "." || filename == "/" || filename == "" {
		c.Render(w, r, "error-message.html", errors.New("invalid filename"))
		return
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, file); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	fileModel, err := models.Files.Insert(&models.File{
		OwnerID:  user.ID,
		FilePath: filename,
		MimeType: handler.Header.Get("Content-Type"),
		Content:  buf.Bytes(),
	})

	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Return JSON if requested (for editor integration)
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"id":       fileModel.ID,
			"url":      c.Host() + "/file/" + fileModel.ID,
			"filename": fileModel.FilePath,
			"mimetype": fileModel.MimeType,
		})
		return
	}

	w.Write([]byte(fileModel.ID))
}

func (c *FilesController) serveFile(w http.ResponseWriter, r *http.Request) {
	file, err := models.Files.Get(r.PathValue("file"))

	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	w.Header().Set("Content-Type", file.MimeType)
	w.Write(file.Content)
}
