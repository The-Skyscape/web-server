package controllers

import (
	"net/http"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/internal/security"
	"www.theskyscape.com/models"
)

func API() (string, *APIController) {
	return "api", &APIController{}
}

type APIController struct {
	application.Controller
}

func (c *APIController) Setup(app *application.App) {
	c.Controller.Setup(app)

	// User endpoints
	http.Handle("GET /api/user", c.ProtectFunc(c.getUser, security.RequireScopes("user:read")))
	http.Handle("GET /api/profile", c.ProtectFunc(c.getProfile, security.RequireScopes("user:read")))

	// Repo endpoints
	http.Handle("GET /api/repos", c.ProtectFunc(c.getRepos, security.RequireScopes("repo:read")))
	http.Handle("GET /api/repos/{id}", c.ProtectFunc(c.getRepo, security.RequireScopes("repo:read")))

	// App endpoints
	http.Handle("GET /api/apps", c.ProtectFunc(c.getApps, security.RequireScopes("app:read")))
	http.Handle("GET /api/apps/{id}", c.ProtectFunc(c.getApp, security.RequireScopes("app:read")))

	// Follow endpoints
	http.Handle("GET /api/followers", c.ProtectFunc(c.getFollowers, security.RequireScopes("follow:read")))
	http.Handle("GET /api/following", c.ProtectFunc(c.getFollowing, security.RequireScopes("follow:read")))
}

func (c APIController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

// Response structs for safe JSON serialization

type UserResponse struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

type ProfileResponse struct {
	ID             string    `json:"id"`
	Handle         string    `json:"handle"`
	Name           string    `json:"name"`
	Avatar         string    `json:"avatar"`
	Description    string    `json:"description"`
	Verified       bool      `json:"verified"`
	FollowersCount int       `json:"followers_count"`
	FollowingCount int       `json:"following_count"`
	ReposCount     int       `json:"repos_count"`
	AppsCount      int       `json:"apps_count"`
	CreatedAt      time.Time `json:"created_at"`
}

type RepoResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Archived    bool      `json:"archived"`
	Owner       *UserResponse `json:"owner"`
	StarsCount  int       `json:"stars_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AppResponse struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Status      string        `json:"status"`
	RepoID      string        `json:"repo_id"`
	Owner       *UserResponse `json:"owner"`
	URL         string        `json:"url"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

type FollowResponse struct {
	ID        string        `json:"id"`
	User      *UserResponse `json:"user"`
	CreatedAt time.Time     `json:"created_at"`
}

// Helper functions to convert models to responses

func userToResponse(u *models.Profile) *UserResponse {
	if u == nil {
		return nil
	}
	return &UserResponse{
		ID:     u.UserID,
		Handle: u.Handle(),
		Name:   u.Name(),
		Avatar: u.Avatar(),
	}
}

func repoToResponse(r *models.Repo) *RepoResponse {
	if r == nil {
		return nil
	}
	var owner *UserResponse
	if o := r.Owner(); o != nil {
		owner = &UserResponse{
			ID:     o.ID,
			Handle: o.Handle,
			Name:   o.Name,
			Avatar: o.Avatar,
		}
	}
	return &RepoResponse{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Archived:    r.Archived,
		Owner:       owner,
		StarsCount:  r.StarsCount(),
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func appToResponse(a *models.App) *AppResponse {
	if a == nil {
		return nil
	}
	var owner *UserResponse
	if o := a.Owner(); o != nil {
		owner = &UserResponse{
			ID:     o.ID,
			Handle: o.Handle,
			Name:   o.Name,
			Avatar: o.Avatar,
		}
	}
	return &AppResponse{
		ID:          a.ID,
		Name:        a.Name,
		Description: a.Description,
		Status:      a.Status,
		RepoID:      a.RepoID,
		Owner:       owner,
		URL:         "https://" + a.ID + ".skysca.pe",
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

func followToResponse(f *models.Follow, profile *models.Profile) *FollowResponse {
	if f == nil {
		return nil
	}
	return &FollowResponse{
		ID:        f.ID,
		User:      userToResponse(profile),
		CreatedAt: f.CreatedAt,
	}
}

// Handlers

func (c *APIController) getUser(w http.ResponseWriter, r *http.Request) {
	user := security.UserFromContext(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	JSON(w, http.StatusOK, &UserResponse{
		ID:     user.ID,
		Handle: user.Handle,
		Name:   user.Name,
		Avatar: user.Avatar,
	})
}

func (c *APIController) getProfile(w http.ResponseWriter, r *http.Request) {
	user := security.UserFromContext(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	profile, err := models.Profiles.Get(user.ID)
	if err != nil {
		JSONError(w, http.StatusNotFound, "profile not found")
		return
	}

	JSON(w, http.StatusOK, &ProfileResponse{
		ID:             profile.ID,
		Handle:         user.Handle,
		Name:           user.Name,
		Avatar:         user.Avatar,
		Description:    profile.Description,
		Verified:       profile.Verified,
		FollowersCount: profile.FollowersCount(),
		FollowingCount: profile.FollowingCount(),
		ReposCount:     profile.ReposCount(),
		AppsCount:      profile.AppsCount(),
		CreatedAt:      profile.CreatedAt,
	})
}

func (c *APIController) getRepos(w http.ResponseWriter, r *http.Request) {
	user := security.UserFromContext(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	repos, err := models.Repos.Search(`
		WHERE OwnerID = ?
		ORDER BY CreatedAt DESC
	`, user.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to fetch repos")
		return
	}

	response := make([]*RepoResponse, 0, len(repos))
	for _, repo := range repos {
		response = append(response, repoToResponse(repo))
	}

	JSON(w, http.StatusOK, response)
}

func (c *APIController) getRepo(w http.ResponseWriter, r *http.Request) {
	user := security.UserFromContext(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	repoID := r.PathValue("id")
	repo, err := models.Repos.Get(repoID)
	if err != nil {
		JSONError(w, http.StatusNotFound, "repo not found")
		return
	}

	// Only allow access to own repos
	if repo.OwnerID != user.ID {
		JSONError(w, http.StatusForbidden, "access denied")
		return
	}

	JSON(w, http.StatusOK, repoToResponse(repo))
}

func (c *APIController) getApps(w http.ResponseWriter, r *http.Request) {
	user := security.UserFromContext(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	apps, err := models.Apps.Search(`
		JOIN repos ON repos.ID = apps.RepoID
		WHERE repos.OwnerID = ? AND apps.Status != 'shutdown'
		ORDER BY apps.CreatedAt DESC
	`, user.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to fetch apps")
		return
	}

	response := make([]*AppResponse, 0, len(apps))
	for _, app := range apps {
		response = append(response, appToResponse(app))
	}

	JSON(w, http.StatusOK, response)
}

func (c *APIController) getApp(w http.ResponseWriter, r *http.Request) {
	user := security.UserFromContext(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	appID := r.PathValue("id")
	app, err := models.Apps.Get(appID)
	if err != nil {
		JSONError(w, http.StatusNotFound, "app not found")
		return
	}

	// Only allow access to own apps
	owner := app.Owner()
	if owner == nil || owner.ID != user.ID {
		JSONError(w, http.StatusForbidden, "access denied")
		return
	}

	JSON(w, http.StatusOK, appToResponse(app))
}

func (c *APIController) getFollowers(w http.ResponseWriter, r *http.Request) {
	user := security.UserFromContext(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	followers, err := models.Follows.Search(`
		WHERE FolloweeID = ?
		ORDER BY CreatedAt DESC
	`, user.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to fetch followers")
		return
	}

	response := make([]*FollowResponse, 0, len(followers))
	for _, follow := range followers {
		profile := follow.Follower()
		response = append(response, followToResponse(follow, profile))
	}

	JSON(w, http.StatusOK, response)
}

func (c *APIController) getFollowing(w http.ResponseWriter, r *http.Request) {
	user := security.UserFromContext(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	following, err := models.Follows.Search(`
		WHERE FollowerID = ?
		ORDER BY CreatedAt DESC
	`, user.ID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to fetch following")
		return
	}

	response := make([]*FollowResponse, 0, len(following))
	for _, follow := range following {
		profile := follow.Followee()
		response = append(response, followToResponse(follow, profile))
	}

	JSON(w, http.StatusOK, response)
}
