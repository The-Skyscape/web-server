package controllers

import (
	"bytes"
	"io"
	"net/http"
	"path/filepath"

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

func (c *FilesController) uploadFile(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	r.ParseMultipartForm(32 << 20)
	file, handler, err := r.FormFile("file")
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	defer file.Close()

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
