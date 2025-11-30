package main

import (
	"embed"
	"log"
	"os"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/controllers"
	"www.theskyscape.com/models"
)

//go:embed all:views
var views embed.FS

//go:embed all:emails
var emails embed.FS

// App timezone - can be changed globally
var appTimezone *time.Location

func init() {
	var err error
	appTimezone, err = time.LoadLocation("America/Los_Angeles")
	if err != nil {
		log.Fatal("Failed to load app timezone:", err)
	}
}

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
		application.WithFunc("format", format),
		application.WithFunc("now", func() time.Time { return time.Now() }),
		application.WithController("auth", auth),
		application.WithController(controllers.Feed()),
		application.WithController(controllers.Profile()),
		application.WithController(controllers.Users()),
		application.WithController(controllers.Repos()),
		application.WithController(controllers.Git()),
		application.WithController(controllers.Files()),
		application.WithController(controllers.Apps()),
		application.WithController(controllers.Comments()),
		application.WithController(controllers.Reactions()),
		application.WithController(controllers.Follows()),
		application.WithController(controllers.Stars()),
		application.WithController(controllers.Messages()),
		application.WithController(controllers.SEO()),
		application.WithController(controllers.OAuth()),
		application.WithController(controllers.API()),
		application.WithController(controllers.Push()),
	)
}

// format converts time to app timezone and formats it
func format(t time.Time, layout string) string {
	return t.In(appTimezone).Format(layout)
}
