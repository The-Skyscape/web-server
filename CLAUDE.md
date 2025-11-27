# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The **web-server** is The Skyscape's social networking platform - a developer-focused social network positioned between LinkedIn and GitHub. It combines professional networking with code hosting, providing:

- User profiles, authentication, and password recovery
- Activity feeds showing community engagement (joins, repo creations, app launches)
- Git repository hosting with web-based file browsing
- Application deployment and management from repositories
- Social features: comments, following, activity streams
- SEO optimization and search functionality

**Module:** `www.theskyscape.com`
**Theme:** DaisyUI "dark" theme
**Architecture:** MVC pattern using devtools framework, HTMX-powered interactivity

## Build & Run

### Building
```bash
# From repository root
make build/website

# Or from web-server directory
go build -o ../build/website .
```

### Running locally
```bash
# Set required environment variables
export DB_URL="http://localhost:8080"           # LibSQL primary database URL
export DB_TOKEN="your-jwt-token"                # Database authentication token
export AUTH_SECRET="your-secret-key"            # JWT signing secret for user sessions
export PORT="5000"                              # Server port (optional, default: 5000)
export PREFIX=""                                # Host prefix for routing (optional)

# Run the server
go run .
```

The server will start on the configured PORT and connect to the LibSQL replica database.

## Architecture

### Database Architecture

Uses **LibSQL embedded replica** pattern:
- **Local replica:** SQLite file at `website.db` synced from primary
- **Primary database:** LibSQL server on headquarters (configured via `DB_URL`)
- **Authentication:** JWT token via `DB_TOKEN` environment variable
- **Read operations:** Served from local SQLite file (fast)
- **Write operations:** Forwarded to primary via HTTP

Database connection defined in `models/database.go`:
```go
DB = remote.Database("website.db", os.Getenv("DB_URL"), os.Getenv("DB_TOKEN"))
```

### MVC Structure

**Models** (`models/` directory):
- `Profile` - User profiles with description, linked to auth users
- `Repo` - Git repositories with owners, stored at `/mnt/git-repos/{id}`
- `App` - Deployed applications linked to repositories (includes OAuth fields)
- `Activity` - Activity feed entries (joined, created, launched, promoted, etc.) with optional Content field
- `Comment` - Comments on repositories
- `File` / `Image` - File metadata and images
- `ResetPasswordToken` - Password recovery tokens
- `OAuthClient` - OAuth 2.0 client credentials for apps
- `OAuthAuthorization` - User authorizations for OAuth apps
- `OAuthAuthorizationCode` - Short-lived authorization codes for OAuth flow
- `Emails` - Email template management

Each model embeds `application.Model` (provides ID, timestamps) and implements `Table()` method.

**Controllers** (`controllers/` directory):
- `Auth` - Authentication, signup, signin, password reset
- `Feed` - Activity feed, homepage, explore page, manifesto
- `Profile` - User profile management
- `Repos` - Repository creation, browsing, commenting, deletion, promotion
- `Files` - File browsing within repositories
- `Git` - Git HTTP server for clone/push/pull operations
- `Apps` - Application deployment from repositories, promotion
- `Comments` - Comment management
- `OAuth` - OAuth 2.0 authorization flow and client management
- `API` - RESTful API endpoints with JWT authentication
- `SEO` - Search engine optimization and metadata

Controllers embed `application.Controller` and follow the devtools pattern:
- Factory function: `func Name() (string, *Controller)`
- `Setup(app *application.App)` - Route registration
- `Handle(r *http.Request) application.Handler` - Request handling with value receiver

**Views** (`views/` directory):
- HTML templates using Go templates and HTMX
- `partials/` - Reusable template components
- `modals/` - Modal dialog templates
- `static/` - Static assets (CSS, JS, images)
- `public/` - Publicly accessible files

**Email Templates** (`emails/` directory):
- `welcome.html` - Welcome email for new users
- `new-user.html` - Notification to existing users about growth
- `password-reset.html` - Password reset instructions
- `partials/` - Email template partials

### Git Repository Hosting

The application provides full Git hosting functionality:

**Storage:**
- Repositories stored as bare Git repos at `/mnt/git-repos/{repo-id}/`
- Initialized with `git init --bare` on creation

**Git HTTP Server:**
- `GitController` uses `github.com/sosedoff/gitkit` library
- Handles `git clone`, `git push`, `git pull` via HTTP
- Routes: `/repo/{repo-id}.git/*`

**Authentication for Git operations:**
- Pull operations: Public (no auth required)
- Push operations: Requires username/password authentication
  - Username: User's handle
  - Password: User's password
  - Only repo owner or admins can push

**File Browsing:**
- Web-based repository file browser
- Syntax highlighting and markdown rendering
- Routes: `/repo/{repo}`, `/repo/{repo}/file/{path...}`

### Activity System

The `Activity` model tracks all user actions for the feed:
- **Actions:** "joined", "created", "launched", etc.
- **Subject types:** "profile", "repo", "app"
- **Subject ID:** References the specific entity

Activities are created automatically:
- Profile creation → `CreateProfile()` in `models/profile.go`
- Repository creation → `NewRepo()` in `models/repo.go`
- App launch → `NewApp()` in `models/app.go`

Feed displays recent activities ordered by `CreatedAt DESC`.

### Application Deployment

Apps can be deployed directly from repositories:

**App lifecycle:**
- Create app from repository (with sanitized ID)
- Launch container with app code
- Monitor status: pending → running → shutdown
- Track errors in `App.Error` field

**Security:**
- App IDs sanitized to prevent command injection: `[^a-z0-9_-]+` removed
- Only alphanumeric, hyphens, and underscores allowed
- See `models/app.go:35` for sanitization logic

## Key Architectural Patterns

### Authentication Flow

1. **Signup/Signin:** Handled by `AuthController` wrapping `authentication.Controller`
2. **Cookie-based sessions:** Cookie name "theskyscape"
3. **Custom handlers:**
   - Signup → Sends welcome emails, creates profile, redirects to `/profile`
   - Signin → Redirects to `next` param or refreshes current page
4. **Optional authentication:** Public pages use `auth.Optional` middleware
5. **Required authentication:** Protected pages use `auth.Required` middleware

### Email System

Email templates loaded asynchronously on startup (`main.go:20`):
```go
go func() {
    if err := models.Emails.LoadTemplates(emails); err != nil {
        log.Fatal("Failed to load email templates:", err)
    }
}()
```

Send emails via `models.Emails.Send()` with template support:
```go
models.Emails.Send(user.Email,
    "Subject Line",
    emailing.WithTemplate("template.html"),
    emailing.WithData("key", value),
)
```

### Search and Discovery

**Repository search** (`controllers/repos.go`):
- Searches across: repo name, description, owner handle
- Uses INNER JOIN with users table
- Excludes archived repositories
- Query parameter: `?query=searchterm`

**App search** (`controllers/apps.go`):
- Searches across: app name, description, repo name, repo description, owner handle
- Uses INNER JOIN with repos and users
- Excludes shutdown apps (`Status != 'shutdown'`)
- Query parameter: `?query=searchterm`

Both support:
- `AllRepos()` / `AllApps()` - All results
- `RecentRepos()` / `RecentApps()` - Limited to 4 most recent

### OAuth 2.0 Integration

The platform implements **OAuth 2.0 Authorization Code flow** allowing deployed apps to access user data via The Skyscape API. This enables apps to integrate with user profiles, repositories, and apps data.

**OAuth Models** (`models/oauth.go`):
- `OAuthClient` - OAuth client credentials per app (bcrypt-hashed secret, redirect URI, allowed scopes)
- `OAuthAuthorization` - User authorizations for apps (tracks scopes, revocation status)
- `OAuthAuthorizationCode` - Short-lived authorization codes (SHA-256 hashed, 10-minute expiration)

**OAuth Controller** (`controllers/oauth.go`):
- `GET /oauth/authorize` - Authorization consent screen
- `POST /oauth/authorize` - Approve/deny authorization
- `POST /oauth/token` - Exchange authorization code for access token
- `GET /app/{app}/users` - View authorized users (app owners only)
- OAuth client management routes (enable, regenerate secret, disable, revoke users)

**API Controller** (`controllers/api.go`):
- `GET /api/user` - Returns authenticated user profile as JSON
- JWT access token validation with revocation checking
- Scopes: `user:read`, `user:write`, `repo:read`, `repo:write`, `app:read`, `app:write`

**OAuth Flow:**
1. App redirects user to `/oauth/authorize?client_id={app_id}&redirect_uri={uri}&response_type=code&scope={scopes}&state={state}`
2. User sees consent screen showing app details and requested permissions
3. User approves → authorization code generated and redirected to app
4. App exchanges code for JWT access token at `/oauth/token` endpoint
5. App uses JWT to call API endpoints (e.g., `GET /api/user`)
6. API validates JWT and checks authorization hasn't been revoked

**Token Security:**
- Authorization codes: SHA-256 hashed, 10-minute expiration, single-use
- Client secrets: bcrypt hashed, regeneratable
- Access tokens: JWT signed with `AUTH_SECRET`, 30-day expiration
- Revocation: Checking authorization table on every API request

**App Owner Controls:**
- Enable OAuth in app settings (generates client ID and secret)
- Regenerate client secret if compromised
- View authorized users with their scopes
- Revoke individual user authorizations
- Disable OAuth completely

**Example Integration (Skykit):**
```go
// Redirect to authorization
redirectURL := fmt.Sprintf(
    "%s/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=user:read&state=%s",
    skyscapeHost, clientID, redirectURI, state,
)

// Exchange code for token
resp := POST("%s/oauth/token", skyscapeHost, {
    "grant_type": "authorization_code",
    "code": authCode,
    "redirect_uri": redirectURI,
    "client_id": clientID,
    "client_secret": clientSecret,
})
accessToken := resp["access_token"]

// Call API
user := GET("%s/api/user", skyscapeHost, {
    "Authorization": "Bearer " + accessToken,
})
```

**Views:**
- `views/authorize.html` - OAuth consent screen with app details and scope descriptions
- `views/app-users.html` - User directory showing authorized users per app
- `views/partials/app-oauth-settings.html` - OAuth settings panel for app owners
- `views/modals/oauth-*.html` - Modals for OAuth client management

## Important File Locations

- **Entry point:** `main.go` - Application initialization and controller registration
- **Database setup:** `models/database.go` - Database connection and model managers
- **Git operations:** `controllers/git.go` - Git HTTP server configuration
- **Auth logic:** `controllers/auth.go` - Custom signup/signin handlers
- **Repository model:** `models/repo.go` - Git repo initialization and file operations
- **App model:** `models/app.go` - Application deployment and ID sanitization
- **Activity model:** `models/activity.go` - Activity feed with promotional content
- **OAuth models:** `models/oauth.go` - OAuth client, authorization, and code models
- **OAuth controller:** `controllers/oauth.go` - OAuth 2.0 authorization flow and management
- **API controller:** `controllers/api.go` - RESTful API with JWT validation
- **JSON helpers:** `controllers/helpers.go` - JSON response utilities
- **Activity feed:** `controllers/feed.go` - Homepage and activity stream

## Environment Variables

**Required:**
- `DB_URL` - LibSQL primary database URL (e.g., `http://hq.skysca.pe:8080`)
- `DB_TOKEN` - JWT token for database authentication
- `AUTH_SECRET` - Secret key for JWT session signing

**Optional:**
- `PORT` - Server port (default: 5000)
- `PREFIX` - Host prefix for routing (used when behind reverse proxy)

## Dependencies

The application depends on:
- `github.com/The-Skyscape/devtools` - Framework (via `../devtools` replace directive)
- `github.com/sosedoff/gitkit` - Git HTTP server
- `github.com/yuin/goldmark` - Markdown rendering
- `golang.org/x/crypto` - Password hashing (bcrypt)
- `github.com/golang-jwt/jwt/v5` - JWT token generation and validation for OAuth

All dependencies managed via `go.mod` with local devtools replacement.

## Controller Registration Pattern

Controllers registered in `main.go` using factory functions:
```go
application.Serve(views,
    application.WithDaisyTheme("dark"),
    application.WithHostPrefix(os.Getenv("PREFIX")),
    application.WithPublicAccess(auth.Optional),       // Set default auth
    application.WithController("auth", auth),           // Named controller
    application.WithController(controllers.Feed()),     // Auto-named
    application.WithController(controllers.Profile()),
    // ... additional controllers
)
```

Named controller ("auth") allows other controllers to reference it via `c.Use("auth")`.

## Template Methods by Controller

Controllers expose public methods that can be called from Go templates. Usage: `{{controllerName.MethodName}}`.

### auth (AuthController)
- `CurrentUser() *authentication.User` - Returns authenticated user (embedded from authentication.Controller)

### feed (FeedController)
- `Page() int` - Current page number (default 1)
- `Limit() int` - Items per page, capped at 100 (default 10)
- `NextPage() int` - Next page number
- `RecentActivities() []*models.Activity` - Paginated activities ordered by date

### profile (ProfileController)
- `CurrentProfile() *models.Profile` - Profile for current user or path param user
- `GetProfile(userID string) *models.Profile` - Profile by user ID
- `RecentProfiles() []*models.Profile` - Up to 4 recent profiles matching search

### repos (ReposController)
- `CurrentRepo() *models.Repo` - Repo from path parameter
- `AllRepos() []*models.Repo` - Non-archived repos matching search
- `RecentRepos() []*models.Repo` - Up to 4 recent repos matching search
- `CurrentFile() *models.Blob` - File being viewed
- `LatestCommit() *models.Commit` - Latest commit on branch
- `FilePath() []PathPart` - Breadcrumb path parts
- `ReadmeFile() *models.Blob` - README.md if exists

### apps (AppsController)
- `CurrentApp() *models.App` - App from path parameter
- `AuthorizedUsers() []*models.OAuthAuthorization` - Users who authorized app
- `AllApps() []*models.App` - Apps matching search (excludes shutdown)
- `RecentApps() []*models.App` - Up to 3 recent apps

### messages (MessagesController)
- `CurrentUser() *models.Profile` - Current user's profile
- `CurrentProfile() *models.Profile` - Conversation recipient profile
- `Count() int` - Unread message count
- `Messages() []*models.Message` - Paginated messages (default 20)
- `Conversations() []*models.Profile` - Profiles user has conversations with
- `UnreadCount() int` - Unread message count
- `Page() int`, `Limit() int`, `NextPage() int` - Pagination

### users (UsersController)
- `AllProfiles() []*models.Profile` - Paginated profiles matching search
- `Page() int`, `Limit() int`, `NextPage() int` - Pagination

### files (FilesController)
- `MyFiles() []*models.File` - Files owned by authenticated user

### oauth (OAuthController)
- `CurrentApp() *models.App` - App for OAuth request (from client_id)
- `RequestedScopes() []string` - Scopes from query param
- `ScopesMatch() bool` - If requested scopes match existing auth
- `AuthorizedUsers() []*models.OAuthAuthorization` - Users who authorized app

### seo (SEOController)
- `Version() string` - Service worker version (Unix timestamp)

## Security Considerations

**Implemented protections:**
- App ID sanitization prevents command injection (see `models/app.go:35`)
- Git push authentication requires valid credentials
- Password hashing via bcrypt
- Admin-only operations checked (e.g., delete posts in `feed.go:71`)

**Git storage location:**
- All repositories at `/mnt/git-repos/` - ensure this mount point exists
- Directory must be writable by server process

## Common Development Tasks

### Adding a new controller
1. Create file in `controllers/` directory
2. Implement factory function returning `(string, *YourController)`
3. Embed `application.Controller`
4. Implement `Setup(app *application.App)` for routes
5. Implement `Handle(r *http.Request) application.Handler`
6. Register in `main.go` via `application.WithController()`

### Adding a new model
1. Create file in `models/` directory
2. Define struct embedding `application.Model`
3. Implement `Table() string` method
4. Create manager in `models/database.go`: `ModelName = database.Manage(DB, new(ModelType))`
5. Add relationship methods as needed (e.g., `User()`, `Repo()`)

### Adding new routes
In controller's `Setup()` method:
```go
http.Handle("GET /path", app.Serve("template.html", auth.Optional))
http.Handle("POST /path", c.ProtectFunc(c.handler, auth.Required))
```

Use `app.Serve()` for rendering templates, `c.ProtectFunc()` for controller methods.

## Real-Time Features

### HTMX Polling Pattern

The application uses HTMX polling for real-time updates without WebSockets. This pattern works well with the devtools framework:

**Feed Polling** (`/feed/poll`):
```html
<!-- Initial poll element in feed.html -->
<div id="feed-poll"
  hx-get="{{host}}/feed/poll?after={{now.Unix}}"
  hx-trigger="every 5s" hx-target="#feed" hx-swap="afterbegin">
</div>
```

**Message Polling** (`/messages/{id}/poll`):
```html
<!-- Poll element updates itself via OOB swap -->
<div id="message-poll"
  hx-get="{{host}}/messages/{{$profile.Handle}}/poll?after={{now.Unix}}"
  hx-trigger="every 3s" hx-target="#messages-container" hx-swap="afterbegin">
</div>
```

**Key Pattern - Out-of-Band Timestamp Updates:**
```html
<!-- In poll response partial (e.g., feed-poll.html) -->
{{range $activities}}
{{template "feed-post.html" .}}
{{end}}

{{if gt (len $activities) 0}}
{{$latest := index $activities (sub (len $activities) 1)}}
<div id="feed-poll"
  hx-get="{{host}}/feed/poll?after={{$latest.CreatedAt.Unix}}"
  hx-trigger="every 5s" hx-target="#feed" hx-swap="afterbegin" hx-swap-oob="true">
</div>
{{end}}
```

The OOB swap (`hx-swap-oob="true"`) updates the poll element with the new timestamp, ensuring the next poll only fetches items newer than the last received.

**Controller Pattern:**
```go
func (c *FeedController) pollFeed(w http.ResponseWriter, r *http.Request) {
    afterStr := r.URL.Query().Get("after")
    var after time.Time
    if afterStr != "" {
        if unix, err := strconv.ParseInt(afterStr, 10, 64); err == nil {
            after = time.Unix(unix, 0)
        }
    }

    activities, _ := models.Activities.Search(`
        WHERE CreatedAt > ?
        ORDER BY CreatedAt ASC
    `, after)

    c.Render(w, r, "feed-poll.html", activities)
}
```

### Push Notifications

Web Push notifications implemented via VAPID:

**Models** (`models/push.go`):
- `PushSubscription` - Stores user's browser push subscription
- `PushNotificationLog` - Tracks rate limiting per (recipient, source) pair

**Rate Limiting:**
- Notifications are rate-limited to 1 per hour per source per recipient
- This prevents spam while allowing notifications from different senders
- When multiple messages arrive, notification aggregates: "You have N new messages"

**Environment Variables:**
- `VAPID_PUBLIC_KEY` - Public key for browser subscription
- `VAPID_PRIVATE_KEY` - Private key for signing push messages

**Usage:**
```go
models.SendPushNotification(
    recipientID,
    sourceID,    // Sender/poster ID for per-conversation throttling
    "Title",
    "Body text",
    "/url/to/open",
)
```

### Service Worker Considerations

The service worker (`views/static/sw.js.html`) caches pages for offline use but must exclude dynamic endpoints:

```javascript
// Skip API, auth, and poll requests - these need fresh data
if (event.request.url.includes('/api/') ||
    event.request.url.includes('/oauth/') ||
    event.request.url.includes('/signin') ||
    event.request.url.includes('/signup') ||
    event.request.url.includes('/poll')) {
  return;
}
```

**Important:** Always add new dynamic endpoints to the skip list to prevent stale cached responses.

## Working with Devtools at Scale

### Framework Strengths

1. **Consistent MVC Pattern**: Every feature follows the same controller/model/view structure, making the codebase predictable and navigable.

2. **Template Method Exposure**: Public controller methods are automatically available in templates via `{{controllerName.MethodName}}`. This reduces boilerplate while maintaining type safety.

3. **Embedded Replicas**: The LibSQL replica pattern provides fast reads with automatic sync. No need to manage complex replication - just call `db.Sync()` when you need fresh data after writes.

4. **Composable Middleware**: Auth middleware (`auth.Required`, `auth.Optional`) chains cleanly with route handlers.

### Common Pitfalls

1. **Profile.ID vs Profile.UserID**: Profiles have both an `ID` (the profile's own ID) and a `UserID` (the auth user's ID). For queries involving profile relationships (messages, follows), use `Profile.ID`, not `Profile.UserID`.

2. **Template Data Requirements**: Email templates often require specific data (e.g., `year` for footer, `user` for personalization). Check existing templates for required fields.

3. **Service Worker Caching**: New dynamic endpoints must be excluded from caching, or users will see stale data.

4. **Value vs Pointer Receivers**: The `Handle()` method uses a value receiver to create a fresh copy per request. Other methods can use pointer receivers.

### Patterns That Scale Well

1. **Background Goroutines for Notifications**: Heavy operations like sending emails/push notifications run in goroutines to not block the response:
   ```go
   go func() {
       for _, follower := range followers {
           models.SendPushNotification(...)
           models.Emails.Send(...)
       }
   }()
   ```

2. **Pagination with Infinite Scroll**: HTMX `hx-trigger="revealed"` combined with `hx-swap="afterend"` creates seamless infinite scroll without breaking the back button.

3. **Per-Entity Rate Limiting**: Rate limits keyed on multiple fields (recipient + source) prevent spam while allowing legitimate notifications from different sources.

4. **Unix Timestamps for URLs**: Using `.Unix()` for timestamps in URLs avoids encoding issues with RFC3339 format (colons cause problems).

### Adding New Real-Time Features

1. Create a poll endpoint in the controller
2. Create a partial template that renders items + updates the poll element via OOB swap
3. Add the initial poll element to the main view
4. Add the poll path to the service worker skip list
5. Consider rate limiting if the feature triggers notifications
