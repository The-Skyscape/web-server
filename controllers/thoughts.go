package controllers

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/models"
)

func Thoughts() (string, *ThoughtsController) {
	return "thoughts", &ThoughtsController{}
}

type ThoughtsController struct {
	application.Controller
}

func (c *ThoughtsController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := app.Use("auth").(*AuthController)

	// Public routes
	http.Handle("GET /thoughts", app.Serve("thoughts.html", auth.Optional))
	http.Handle("GET /thought/{thought}", c.ProtectFunc(c.view, auth.Optional))
	http.Handle("GET /user/{user}/thoughts", app.Serve("user-thoughts.html", auth.Optional))

	// Authenticated routes
	http.Handle("GET /thoughts/new", app.Serve("thought-edit.html", auth.Required))
	http.Handle("GET /thought/{thought}/edit", app.Serve("thought-edit.html", auth.Required))
	http.Handle("POST /thoughts", c.ProtectFunc(c.create, auth.Required))
	http.Handle("POST /thought/{thought}", c.ProtectFunc(c.update, auth.Required))
	http.Handle("DELETE /thought/{thought}", c.ProtectFunc(c.delete, auth.Required))

	// Social features
	http.Handle("POST /thought/{thought}/star", c.ProtectFunc(c.star, auth.Required))
	http.Handle("DELETE /thought/{thought}/star", c.ProtectFunc(c.unstar, auth.Required))

	// Block management endpoints (HTMX)
	http.Handle("POST /thought/{thought}/header", c.ProtectFunc(c.uploadHeader, auth.Required))
	http.Handle("POST /thought/{thought}/blocks", c.ProtectFunc(c.createBlock, auth.Required))
	http.Handle("POST /thought/{thought}/blocks/image", c.ProtectFunc(c.createImageBlock, auth.Required))
	http.Handle("POST /thought/{thought}/blocks/reorder", c.ProtectFunc(c.reorderBlocks, auth.Required))
	http.Handle("POST /thought/{thought}/block/{block}", c.ProtectFunc(c.updateBlock, auth.Required))
	http.Handle("DELETE /thought/{thought}/block/{block}", c.ProtectFunc(c.deleteBlock, auth.Required))
}

func (c ThoughtsController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

// CurrentThought returns the thought from the URL path
func (c *ThoughtsController) CurrentThought() *models.Thought {
	id := c.PathValue("thought")
	if id == "" {
		return nil
	}
	thought, err := models.Thoughts.Get(id)
	if err != nil {
		return nil
	}
	return thought
}

// CurrentProfile returns the profile for the user path parameter
func (c *ThoughtsController) CurrentProfile() *models.Profile {
	handle := c.PathValue("user")
	if handle == "" {
		return nil
	}
	user, err := models.Auth.LookupUser(handle)
	if err != nil {
		return nil
	}
	profile, _ := models.Profiles.First("WHERE UserID = ?", user.ID)
	return profile
}

// AllThoughts returns all published thoughts
func (c *ThoughtsController) AllThoughts() []*models.Thought {
	thoughts, _ := models.Thoughts.Search(`
		WHERE Published = true
		ORDER BY CreatedAt DESC
	`)
	return thoughts
}

// RecentThoughts returns recent published thoughts (limited)
func (c *ThoughtsController) RecentThoughts() []*models.Thought {
	thoughts, _ := models.Thoughts.Search(`
		WHERE Published = true
		ORDER BY CreatedAt DESC
		LIMIT 10
	`)
	return thoughts
}

// UserThoughts returns thoughts by a specific user
func (c *ThoughtsController) UserThoughts(userID string) []*models.Thought {
	thoughts, _ := models.Thoughts.Search(`
		WHERE UserID = ? AND Published = true
		ORDER BY CreatedAt DESC
	`, userID)
	return thoughts
}

// MyThoughts returns the current user's thoughts (including drafts)
func (c *ThoughtsController) MyThoughts() []*models.Thought {
	auth := c.Use("auth").(*AuthController)
	user := auth.CurrentUser()
	if user == nil {
		return nil
	}
	thoughts, _ := models.Thoughts.Search(`
		WHERE UserID = ?
		ORDER BY CreatedAt DESC
	`, user.ID)
	return thoughts
}

// view handles viewing a thought and recording the view
func (c *ThoughtsController) view(w http.ResponseWriter, r *http.Request) {
	thought, err := models.Thoughts.Get(r.PathValue("thought"))
	if err != nil {
		c.RenderError(w, r, application.ErrNotFound)
		return
	}

	// Only allow viewing published thoughts (unless owner)
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(r)
	if !thought.Published && (user == nil || user.ID != thought.UserID) {
		c.RenderError(w, r, application.ErrNotFound)
		return
	}

	// Record view
	var userID string
	if user != nil {
		userID = user.ID
	}
	thought.RecordView(userID, r.RemoteAddr)

	c.Render(w, r, "thought.html", thought)
}

// create handles creating a new thought
func (c *ThoughtsController) create(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	published := r.FormValue("published") == "true"

	if title == "" {
		c.Render(w, r, "error-message.html", errors.New("title is required"))
		return
	}

	if len(title) > 200 {
		c.Render(w, r, "error-message.html", errors.New("title too long, max 200 characters"))
		return
	}

	// Generate slug from title
	slug := generateSlug(title)

	thought := &models.Thought{
		UserID:    user.ID,
		Title:     title,
		Slug:      slug,
		Published: published,
	}

	created, err := models.Thoughts.Insert(thought)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Create activity if published
	if published {
		models.Activities.Insert(&models.Activity{
			UserID:      user.ID,
			Action:      "published",
			SubjectType: "thought",
			SubjectID:   created.ID,
		})
	}

	// Redirect to edit page for the block editor
	c.Redirect(w, r, "/thought/"+created.ID+"/edit")
}

// update handles updating a thought
func (c *ThoughtsController) update(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	thought, err := models.Thoughts.Get(r.PathValue("thought"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("thought not found"))
		return
	}

	if thought.UserID != user.ID && !user.IsAdmin {
		c.Render(w, r, "error-message.html", errors.New("not authorized"))
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	published := r.FormValue("published") == "true"

	if title == "" {
		c.Render(w, r, "error-message.html", errors.New("title is required"))
		return
	}

	wasPublished := thought.Published
	thought.Title = title
	thought.Published = published
	thought.Slug = generateSlug(title)

	if err := models.Thoughts.Update(thought); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Create activity if newly published
	if published && !wasPublished {
		models.Activities.Insert(&models.Activity{
			UserID:      user.ID,
			Action:      "published",
			SubjectType: "thought",
			SubjectID:   thought.ID,
		})
	}

	c.Redirect(w, r, "/thought/"+thought.ID)
}

// delete handles deleting a thought
func (c *ThoughtsController) delete(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	thought, err := models.Thoughts.Get(r.PathValue("thought"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("thought not found"))
		return
	}

	if thought.UserID != user.ID && !user.IsAdmin {
		c.Render(w, r, "error-message.html", errors.New("not authorized"))
		return
	}

	if err := models.Thoughts.Delete(thought); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Redirect(w, r, "/profile")
}

// star handles starring a thought
func (c *ThoughtsController) star(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	thought, err := models.Thoughts.Get(r.PathValue("thought"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("thought not found"))
		return
	}

	// Check if already starred
	if thought.IsStarredBy(user.ID) {
		c.Refresh(w, r)
		return
	}

	// Create star
	models.ThoughtStars.Insert(&models.ThoughtStar{
		ThoughtID: thought.ID,
		UserID:    user.ID,
	})

	// Update cached count
	thought.StarsCount++
	models.Thoughts.Update(thought)

	c.Refresh(w, r)
}

// unstar handles unstarring a thought
func (c *ThoughtsController) unstar(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	thought, err := models.Thoughts.Get(r.PathValue("thought"))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("thought not found"))
		return
	}

	// Find and delete star
	star, err := models.ThoughtStars.First("WHERE ThoughtID = ? AND UserID = ?", thought.ID, user.ID)
	if err != nil {
		c.Refresh(w, r)
		return
	}

	models.ThoughtStars.Delete(star)

	// Update cached count
	if thought.StarsCount > 0 {
		thought.StarsCount--
		models.Thoughts.Update(thought)
	}

	c.Refresh(w, r)
}

// generateSlug creates a URL-friendly slug from a title
func generateSlug(title string) string {
	slug := strings.ToLower(title)
	slug = regexp.MustCompile(`[^a-z0-9\s-]`).ReplaceAllString(slug, "")
	slug = regexp.MustCompile(`\s+`).ReplaceAllString(slug, "-")
	slug = regexp.MustCompile(`-+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 100 {
		slug = slug[:100]
	}
	return slug
}

// Block management handlers

// createBlock creates a new block for a thought
func (c *ThoughtsController) createBlock(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	thought, err := models.Thoughts.Get(r.PathValue("thought"))
	if err != nil {
		c.RenderError(w, r, application.ErrNotFound)
		return
	}

	if thought.UserID != user.ID && !user.IsAdmin {
		c.RenderError(w, r, application.ErrForbidden)
		return
	}

	// Parse form data
	blockType := r.FormValue("type")
	if blockType == "" {
		blockType = "paragraph"
	}
	position, _ := strconv.Atoi(r.FormValue("position"))
	content := r.FormValue("content")
	fileID := r.FormValue("file_id")

	// Validate type
	validTypes := map[string]bool{
		"paragraph": true, "heading": true, "quote": true,
		"code": true, "list": true, "image": true, "file": true,
	}
	if !validTypes[blockType] {
		c.RenderError(w, r, errors.New("invalid block type"))
		return
	}

	// If position not specified, add at end
	if position == 0 {
		position = models.ThoughtBlocks.Count("WHERE ThoughtID = ?", thought.ID) + 1
	}

	// Shift existing blocks if inserting in middle
	existingBlocks := thought.Blocks()
	for _, block := range existingBlocks {
		if block.Position >= position {
			block.Position++
			models.ThoughtBlocks.Update(block)
		}
	}

	block := &models.ThoughtBlock{
		ThoughtID: thought.ID,
		Type:      blockType,
		Content:   content,
		FileID:    fileID,
		Position:  position,
	}

	created, err := models.ThoughtBlocks.Insert(block)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Return the rendered block HTML
	c.Render(w, r, "editor-block.html", created)
}

// updateBlock updates a block's content
func (c *ThoughtsController) updateBlock(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	thought, err := models.Thoughts.Get(r.PathValue("thought"))
	if err != nil {
		c.RenderError(w, r, application.ErrNotFound)
		return
	}

	if thought.UserID != user.ID && !user.IsAdmin {
		c.RenderError(w, r, application.ErrForbidden)
		return
	}

	block, err := models.ThoughtBlocks.Get(r.PathValue("block"))
	if err != nil || block.ThoughtID != thought.ID {
		c.RenderError(w, r, application.ErrNotFound)
		return
	}

	// Update from form data - always update content since empty is valid
	r.ParseForm()
	if _, hasContent := r.Form["content"]; hasContent {
		block.Content = r.FormValue("content")
	}
	if blockType := r.FormValue("type"); blockType != "" {
		block.Type = blockType
	}
	if fileID := r.FormValue("file_id"); fileID != "" {
		block.FileID = fileID
	}

	if err := models.ThoughtBlocks.Update(block); err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Return empty response for hx-swap="none"
	w.WriteHeader(http.StatusOK)
}

// deleteBlock removes a block from a thought
func (c *ThoughtsController) deleteBlock(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	thought, err := models.Thoughts.Get(r.PathValue("thought"))
	if err != nil {
		c.RenderError(w, r, application.ErrNotFound)
		return
	}

	if thought.UserID != user.ID && !user.IsAdmin {
		c.RenderError(w, r, application.ErrForbidden)
		return
	}

	block, err := models.ThoughtBlocks.Get(r.PathValue("block"))
	if err != nil || block.ThoughtID != thought.ID {
		c.RenderError(w, r, application.ErrNotFound)
		return
	}

	deletedPosition := block.Position

	if err := models.ThoughtBlocks.Delete(block); err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Reorder remaining blocks
	remainingBlocks := thought.Blocks()
	for _, b := range remainingBlocks {
		if b.Position > deletedPosition {
			b.Position--
			models.ThoughtBlocks.Update(b)
		}
	}

	// Return empty response - HTMX will remove the element with hx-swap="outerHTML"
	w.WriteHeader(http.StatusOK)
}

// createImageBlock handles image upload and creates an image block in one request
func (c *ThoughtsController) createImageBlock(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	thought, err := models.Thoughts.Get(r.PathValue("thought"))
	if err != nil {
		c.RenderError(w, r, application.ErrNotFound)
		return
	}

	if thought.UserID != user.ID && !user.IsAdmin {
		c.RenderError(w, r, application.ErrForbidden)
		return
	}

	// Parse multipart form
	r.ParseMultipartForm(maxImageSize)
	file, handler, err := r.FormFile("file")
	if err != nil {
		c.RenderError(w, r, errors.New("no file uploaded"))
		return
	}
	defer file.Close()

	// Validate file size
	if handler.Size > maxImageSize {
		c.RenderError(w, r, errors.New("image too large, max 10MB"))
		return
	}

	// Validate it's an image
	mimeType := handler.Header.Get("Content-Type")
	if !strings.HasPrefix(mimeType, "image/") {
		c.RenderError(w, r, errors.New("file must be an image"))
		return
	}

	// Sanitize filename
	filename := filepath.Base(filepath.Clean(handler.Filename))
	if filename == "." || filename == "/" || filename == "" {
		filename = "image"
	}

	// Read file content
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, file); err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Create file record
	fileModel, err := models.Files.Insert(&models.File{
		OwnerID:  user.ID,
		FilePath: filename,
		MimeType: mimeType,
		Content:  buf.Bytes(),
	})
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Get next position
	position := models.ThoughtBlocks.Count("WHERE ThoughtID = ?", thought.ID) + 1

	// Create image block
	block := &models.ThoughtBlock{
		ThoughtID: thought.ID,
		Type:      "image",
		Content:   "", // Caption can be added later
		FileID:    fileModel.ID,
		Position:  position,
	}

	created, err := models.ThoughtBlocks.Insert(block)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Return the rendered block HTML
	c.Render(w, r, "editor-block.html", created)
}

// uploadHeader handles uploading a header image for a thought
func (c *ThoughtsController) uploadHeader(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	thought, err := models.Thoughts.Get(r.PathValue("thought"))
	if err != nil {
		c.RenderError(w, r, application.ErrNotFound)
		return
	}

	if thought.UserID != user.ID && !user.IsAdmin {
		c.RenderError(w, r, application.ErrForbidden)
		return
	}

	// Parse multipart form
	r.ParseMultipartForm(maxImageSize)
	file, handler, err := r.FormFile("file")
	if err != nil {
		c.RenderError(w, r, errors.New("no file uploaded"))
		return
	}
	defer file.Close()

	// Validate it's an image
	mimeType := handler.Header.Get("Content-Type")
	if !strings.HasPrefix(mimeType, "image/") {
		c.RenderError(w, r, errors.New("file must be an image"))
		return
	}

	// Sanitize filename
	filename := filepath.Base(filepath.Clean(handler.Filename))
	if filename == "." || filename == "/" || filename == "" {
		filename = "header"
	}

	// Read file content
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, file); err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Create file record
	fileModel, err := models.Files.Insert(&models.File{
		OwnerID:  user.ID,
		FilePath: filename,
		MimeType: mimeType,
		Content:  buf.Bytes(),
	})
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Update thought header
	thought.HeaderImageID = fileModel.ID
	if err := models.Thoughts.Update(thought); err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Return updated header partial
	c.Render(w, r, "thought-header-image.html", thought)
}

// reorderBlocks updates block positions after drag-and-drop
func (c *ThoughtsController) reorderBlocks(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	thought, err := models.Thoughts.Get(r.PathValue("thought"))
	if err != nil {
		c.RenderError(w, r, application.ErrNotFound)
		return
	}

	if thought.UserID != user.ID && !user.IsAdmin {
		c.RenderError(w, r, application.ErrForbidden)
		return
	}

	// Get the block IDs in their new order from form data
	// Format: order[]=id1&order[]=id2&order[]=id3
	r.ParseForm()
	blockIDs := r.Form["order[]"]
	if len(blockIDs) == 0 {
		// Try alternative format: order=id1,id2,id3
		if orderStr := r.FormValue("order"); orderStr != "" {
			blockIDs = strings.Split(orderStr, ",")
		}
	}

	if len(blockIDs) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Update each block's position
	for i, blockID := range blockIDs {
		block, err := models.ThoughtBlocks.Get(blockID)
		if err != nil || block.ThoughtID != thought.ID {
			continue // Skip invalid blocks
		}
		block.Position = i + 1
		models.ThoughtBlocks.Update(block)
	}

	w.WriteHeader(http.StatusOK)
}
