package main

import (
	"embed"
	"log"
	"os"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/controllers"
	"www.theskyscape.com/models"
)

//go:embed all:views
var views embed.FS

//go:embed all:emails
var emails embed.FS

func main() {
	go func() {
		if err := models.Emails.LoadTemplates(emails); err != nil {
			log.Fatal("Failed to load email templates:", err)
		}
	}()

	_, auth := controllers.Auth()
	application.Serve(views,
		application.WithDaisyTheme("dark"),
		application.WithHostPrefix(os.Getenv("PREFIX")),
		application.WithPublicAccess(auth.Optional),
		application.WithController("auth", auth),
		application.WithController(controllers.Feed()),
		application.WithController(controllers.Profile()),
		application.WithController(controllers.Repos()),
		application.WithController(controllers.Git()),
		application.WithController(controllers.Files()),
		application.WithController(controllers.Apps()),
		application.WithController(controllers.Comments()),
	)
}
