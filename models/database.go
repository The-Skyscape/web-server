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
	Files      = database.Manage(DB, new(File))
	Images     = database.Manage(DB, new(Image))

	PasswordResetTokens = database.Manage(DB, new(ResetPasswordToken))

	OAuthAuthorizations     = database.Manage(DB, new(OAuthAuthorization))
	OAuthAuthorizationCodes = database.Manage(DB, new(OAuthAuthorizationCode))
)
