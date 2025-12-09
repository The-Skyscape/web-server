package controllers

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/emailing"
	"www.theskyscape.com/models"
)

func Feed() (string, *FeedController) {
	return "feed", &FeedController{
		defaultPage:  1,
		defaultLimit: 10,
	}
}

type FeedController struct {
	application.Controller
	defaultPage  int
	defaultLimit int
}

func (c *FeedController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := c.Use("auth").(*AuthController)

	http.Handle("/", app.Serve("tbd.html", auth.Required))
	http.Handle("/{$}", app.ProtectFunc(c.serveFeed, auth.Optional))
	http.Handle("/explore", app.Serve("explore.html", auth.Optional))
	http.Handle("/manifesto", app.Serve("manifesto.html", auth.Optional))
	http.Handle("GET /feed/poll", c.ProtectFunc(c.pollFeed, auth.Optional))
	http.Handle("POST /feed/post", c.ProtectFunc(c.createPost, auth.Required))
	http.Handle("DELETE /feed/{post}", c.ProtectFunc(c.deletePost, auth.Required))
	http.Handle("GET /post/{post}", app.Serve("post.html", auth.Optional))
}

func (c FeedController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *FeedController) serveFeed(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	if user, _, _ := auth.Authenticate(r); user == nil {
		c.Render(w, r, "signup.html", nil)
		return
	}

	profile := c.Use("profile").(*ProfileController)
	profile.Request = r
	if profile.CurrentProfile() == nil {
		c.Render(w, r, "setup.html", nil)
		return
	}

	c.Render(w, r, "feed.html", nil)
}

func (c *FeedController) CurrentPost() *models.Activity {
	post, _ := models.Activities.Get(c.PathValue("post"))
	return post
}

func (c *FeedController) Page() int {
	return ParsePage(c.URL.Query(), c.defaultPage)
}

func (c *FeedController) Limit() int {
	return ParseLimit(c.URL.Query(), c.defaultLimit)
}

func (c *FeedController) NextPage() int {
	return c.Page() + 1
}

func (c *FeedController) RecentActivities() []*models.Activity {
	page := c.Page()
	limit := c.Limit()
	offset := (page - 1) * limit

	activities, _ := models.Activities.Search(`
		ORDER BY CreatedAt DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	return activities
}

// PersonalizedActivities returns activities from followed users + own posts
func (c *FeedController) PersonalizedActivities() []*models.Activity {
	auth := c.Use("auth").(*AuthController)
	user := auth.CurrentUser()
	if user == nil {
		return c.RecentActivities() // Fallback to global feed for logged out users
	}

	profile, _ := models.Profiles.First("WHERE UserID = ?", user.ID)
	if profile == nil {
		return c.RecentActivities()
	}

	// Build list of user IDs: own ID + all followed user IDs
	following := profile.Following()
	userIDs := make([]interface{}, 0, len(following)+1)
	userIDs = append(userIDs, user.ID)
	for _, f := range following {
		userIDs = append(userIDs, f.FolloweeID)
	}

	page := c.Page()
	limit := c.Limit()
	offset := (page - 1) * limit

	// Build placeholder string for IN clause
	placeholders := "?"
	for i := 1; i < len(userIDs); i++ {
		placeholders += ",?"
	}

	args := append(userIDs, limit, offset)
	activities, _ := models.Activities.Search(`
		WHERE UserID IN (`+placeholders+`)
		ORDER BY CreatedAt DESC
		LIMIT ? OFFSET ?
	`, args...)

	return activities
}

// ActivePromotions returns all non-expired promotions
func (c *FeedController) ActivePromotions() []*models.Promotion {
	return models.ActivePromotions()
}

// FeedItem represents an Activity, Promotion, or end-of-feed marker
type FeedItem struct {
	Activity  *models.Activity
	Promotion *models.Promotion
	EndOfFeed bool
}

// IsPromotion returns true if this is a promotion item
func (f FeedItem) IsPromotion() bool {
	return f.Promotion != nil
}

// IsEndOfFeed returns true if this marks the end of the feed
func (f FeedItem) IsEndOfFeed() bool {
	return f.EndOfFeed
}

// FeedWithPromotions returns personalized activities with 1 promotion per page
// The promotion is positioned in the middle of the activities (at limit/2)
// Appends an EndOfFeed marker when there are no more activities to load
func (c *FeedController) FeedWithPromotions() []FeedItem {
	activities := c.PersonalizedActivities()
	promotions := c.ActivePromotions()
	limit := c.Limit()
	isEndOfFeed := len(activities) < limit

	numPromos := len(promotions)
	if numPromos == 0 {
		// No promotions available, return activities only
		result := make([]FeedItem, 0, len(activities)+1)
		for _, activity := range activities {
			result = append(result, FeedItem{Activity: activity})
		}
		if isEndOfFeed {
			result = append(result, FeedItem{EndOfFeed: true})
		}
		return result
	}

	result := make([]FeedItem, 0, len(activities)+2)

	// Rotate through promotions based on page number
	page := c.Page()
	promo := promotions[(page-1)%numPromos]

	// Insert promotion in the middle of activities
	promoPosition := limit / 2

	for i, activity := range activities {
		// Insert promotion at the middle position
		if i == promoPosition {
			result = append(result, FeedItem{Promotion: promo})
		}
		result = append(result, FeedItem{Activity: activity})
	}

	// If fewer activities than promoPosition, add promotion at the end
	if len(activities) <= promoPosition && len(activities) > 0 {
		result = append(result, FeedItem{Promotion: promo})
	}

	// Add end-of-feed marker when no more activities to load
	if isEndOfFeed {
		result = append(result, FeedItem{EndOfFeed: true})
	}

	return result
}

// MyRepos returns all repos owned by the current user for the promote dropdown
func (c *FeedController) MyRepos() []*models.Repo {
	auth := c.Use("auth").(*AuthController)
	user := auth.CurrentUser()
	if user == nil {
		return nil
	}
	repos, _ := models.Repos.Search(`
		WHERE OwnerID = ? AND Archived = false
		ORDER BY CreatedAt DESC
	`, user.ID)
	return repos
}

// MyApps returns all apps owned by the current user for the promote dropdown
func (c *FeedController) MyApps() []*models.App {
	auth := c.Use("auth").(*AuthController)
	user := auth.CurrentUser()
	if user == nil {
		return nil
	}
	apps, _ := models.Apps.Search(`
		JOIN repos ON repos.ID = apps.RepoID
		WHERE repos.OwnerID = ? AND apps.Status != 'shutdown'
		ORDER BY apps.CreatedAt DESC
	`, user.ID)
	return apps
}

// pollFeed returns new activities since the given timestamp (filtered by followed users)
func (c *FeedController) pollFeed(w http.ResponseWriter, r *http.Request) {
	// Parse the 'after' timestamp (Unix seconds)
	afterStr := r.URL.Query().Get("after")
	var after time.Time
	if afterStr != "" {
		if unix, err := strconv.ParseInt(afterStr, 10, 64); err == nil {
			after = time.Unix(unix, 0)
		}
	}

	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)

	if err != nil {
		c.Refresh(w, r)
		return
	}

	var activities []*models.Activity

	if user == nil {
		// Fallback to global feed for logged out users
		activities, _ = models.Activities.Search(`
			WHERE CreatedAt > ?
			ORDER BY CreatedAt ASC
		`, after)
	} else {
		profile, _ := models.Profiles.First("WHERE UserID = ?", user.ID)
		if profile == nil {
			activities, _ = models.Activities.Search(`
				WHERE CreatedAt > ?
				ORDER BY CreatedAt ASC
			`, after)
		} else {
			// Build list of user IDs: own ID + all followed user IDs
			following := profile.Following()
			userIDs := make([]interface{}, 0, len(following)+1)
			userIDs = append(userIDs, user.ID)
			for _, f := range following {
				userIDs = append(userIDs, f.FolloweeID)
			}

			// Build placeholder string for IN clause
			placeholders := "?"
			for i := 1; i < len(userIDs); i++ {
				placeholders += ",?"
			}

			args := append(userIDs, after)
			activities, _ = models.Activities.Search(`
				WHERE UserID IN (`+placeholders+`) AND CreatedAt > ?
				ORDER BY CreatedAt ASC
			`, args...)
		}
	}

	c.Render(w, r, "feed-poll.html", activities)
}

const maxImageSize = 10 * 1024 * 1024 // 10MB

var allowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

func (c *FeedController) createPost(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	r.ParseMultipartForm(maxImageSize)

	content := r.FormValue("content")
	if content == "" {
		c.Render(w, r, "error-message.html", errors.New("Post content cannot be empty"))
		return
	}
	if len(content) > MaxContentLength {
		c.Render(w, r, "error-message.html", errors.New("Post content too long"))
		return
	}

	// Handle repo/app promotion
	var subjectType, subjectID string
	if repoID := r.FormValue("repo_id"); repoID != "" {
		subjectType = "repo"
		subjectID = repoID
	} else if appID := r.FormValue("app_id"); appID != "" {
		subjectType = "app"
		subjectID = appID
	}

	// Handle file upload
	var fileID string
	if file, handler, err := r.FormFile("image"); err == nil {
		defer file.Close()

		if handler.Size > maxImageSize {
			c.Render(w, r, "error-message.html", errors.New("Image too large, max 10MB"))
			return
		}

		mimeType := handler.Header.Get("Content-Type")
		if !allowedImageTypes[mimeType] {
			c.Render(w, r, "error-message.html", errors.New("Only images are allowed"))
			return
		}

		var buf bytes.Buffer
		if _, err := io.Copy(&buf, file); err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}

		fileModel, err := models.Files.Insert(&models.File{
			OwnerID:  user.ID,
			FilePath: handler.Filename,
			MimeType: mimeType,
			Content:  buf.Bytes(),
		})
		if err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}
		fileID = fileModel.ID
	}

	_, err = models.Activities.Insert(&models.Activity{
		UserID:      user.ID,
		Action:      "posted",
		SubjectType: subjectType,
		SubjectID:   subjectID,
		Content:     content,
		FileID:      fileID,
	})
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Notify followers in background
	go func() {
		poster, _ := models.Profiles.Get(user.ID)
		if poster == nil {
			return
		}

		preview := content
		if len(preview) > 200 {
			preview = preview[:197] + "..."
		}

		for _, follow := range poster.Followers() {
			follower := follow.Follower()
			if follower == nil {
				continue
			}
			followerUser := follower.User()
			if followerUser == nil {
				continue
			}

			// Send push notification
			models.SendPushNotification(
				follower.ID,
				poster.ID, // source = poster
				"New post from @"+poster.Handle(),
				preview,
				"/",
			)

			// Send email notification
			models.Emails.Send(followerUser.Email,
				"New post from "+poster.Name(),
				emailing.WithTemplate("new-post.html"),
				emailing.WithData("poster", poster),
				emailing.WithData("recipient", follower),
				emailing.WithData("user", followerUser),
				emailing.WithData("preview", preview),
				emailing.WithData("year", time.Now().Year()),
			)
		}
	}()

	c.Refresh(w, r)
}

func (c *FeedController) deletePost(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	post, err := models.Activities.Get(r.PathValue("post"))
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	if !user.IsAdmin && post.UserID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("Not allowed"))
		return
	}

	if err = models.Activities.Delete(post); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}
