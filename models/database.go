package models

import (
	"os"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/The-Skyscape/devtools/pkg/database/remote"
)

var (
	DB = remote.Database("website.db", os.Getenv("DB_URL"), os.Getenv("DB_TOKEN"))

	Auth       = authentication.Manage(DB)
	Profiles   = database.Manage(DB, new(Profile))
	Repos      = database.Manage(DB, new(Repo))
	Apps       = database.Manage(DB, new(App))
	Activities = database.Manage(DB, new(Activity))
	Comments   = database.Manage(DB, new(Comment))
	Follows    = database.Manage(DB, new(Follow))
	Stars      = database.Manage(DB, new(Star))
	Files      = database.Manage(DB, new(File))
	Images     = database.Manage(DB, new(Image))
	Reactions  = database.Manage(DB, new(Reaction))
	Promotions = database.Manage(DB, new(Promotion))

	PasswordResetTokens  = database.Manage(DB, new(ResetPasswordToken))
	RateLimits           = database.Manage(DB, new(RateLimit))
	Messages             = database.Manage(DB, new(Message))
	PushSubscriptions    = database.Manage(DB, new(PushSubscription))
	PushNotificationLogs = database.Manage(DB, new(PushNotificationLog))

	OAuthAuthorizations     = database.Manage(DB, new(OAuthAuthorization))
	OAuthAuthorizationCodes = database.Manage(DB, new(OAuthAuthorizationCode))

	AppMetricsManager = database.Manage(DB, new(AppMetrics))

	Thoughts      = database.Manage(DB, new(Thought))
	ThoughtViews  = database.Manage(DB, new(ThoughtView))
	ThoughtStars  = database.Manage(DB, new(ThoughtStar))
	ThoughtBlocks = database.Manage(DB, new(ThoughtBlock))
)
