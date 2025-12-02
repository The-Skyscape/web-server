package controllers

import (
	"errors"
	"net/http"
	"regexp"
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
		c.RenderError(w, r, errors.New("thought not found"))
		return
	}

	// Only allow viewing published thoughts (unless owner)
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(r)
	if !thought.Published && (user == nil || user.ID != thought.UserID) {
		c.RenderError(w, r, errors.New("thought not found"))
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
	content := r.FormValue("content")
	published := r.FormValue("published") == "true"

	if title == "" {
		c.Render(w, r, "error-message.html", errors.New("title is required"))
		return
	}

	if len(title) > 200 {
		c.Render(w, r, "error-message.html", errors.New("title too long, max 200 characters"))
		return
	}

	if len(content) > 100000 {
		c.Render(w, r, "error-message.html", errors.New("content too long"))
		return
	}

	// Generate slug from title
	slug := generateSlug(title)

	thought := &models.Thought{
		UserID:    user.ID,
		Title:     title,
		Content:   content,
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

	c.Redirect(w, r, "/thought/"+created.ID)
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
	content := r.FormValue("content")
	published := r.FormValue("published") == "true"

	if title == "" {
		c.Render(w, r, "error-message.html", errors.New("title is required"))
		return
	}

	wasPublished := thought.Published
	thought.Title = title
	thought.Content = content
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
