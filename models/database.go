package models

import (
	"os"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/The-Skyscape/devtools/pkg/database/remote"
)

var (
	url, token = os.Getenv("DB_URL"), os.Getenv("DB_TOKEN")

	DB = remote.Database("website.db", url, token)

	Auth = authentication.Manage(DB)

	Profiles = database.Manage(DB, new(Profile))

	Repos = database.Manage(DB, new(Repo))

	Activities = database.Manage(DB, new(Activity))
)
